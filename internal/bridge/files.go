package bridge

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Images the page sends into a conversation (POST /api/files/<session>): kept in the session's own working folder,
// .symphony/inbox/ (a file inside the folder the session works in, its Read tool reads without asking; one outside
// would want an approval for every picture), named by an id the bridge makes (never by the upload's name) with the
// extension of the type the bytes sniff as (png, jpeg, gif or webp, nothing else), and served back to the page at
// /api/files/<session>/<id>. The session gets the file's path in its prompt and reads it: the mod API gives a plugin
// no way to hand the engine an image's bytes. .symphony/ goes in the repository's own exclude file (gitExclude), so
// nothing is committed.

const fileMax = 10 << 20 // bytes an image may have

var (
	ErrFileNotImage = newError("file_not_image", "not an image (png, jpeg, gif or webp)")
	ErrFileTooBig   = newError("file_too_big", "the image is over 10 MiB")
	ErrFileUnknown  = newError("file_unknown", "no such file")
	errFilesNowhere = newError("files_nowhere", "the session named no working folder to keep the file in")
	fileID          = regexp.MustCompile(`^[0-9a-f]{8,}\.(png|jpg|gif|webp)$`)
	imageExt        = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}
)

// SavedFile is what an upload answers: the id the file is served and named by, and where it is on disk.
type SavedFile struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"` // the upload's own name, for the page to show
	URL  string `json:"url"`            // /api/files/<session>/<id>
	Size int64  `json:"size"`
	Path string `json:"-"` // on disk, for the session's prompt
}

// filesDir is where a session's files go: .symphony/inbox in its working folder; "" (and the error) for a session the
// bridge doesn't know or one that named no folder.
func (h *Hub) filesDir(session string) (string, string, error) {
	h.mu.Lock()
	s := h.sessions[session]
	cwd := ""
	if s != nil {
		cwd = s.Cwd
	}
	h.mu.Unlock()
	if s == nil {
		return "", "", ErrSessionOffline
	}
	if cwd == "" {
		return "", "", errFilesNowhere
	}
	return filepath.Join(cwd, ".symphony", "inbox"), cwd, nil
}

// SaveFile keeps an image the page sends for a session: the body's first bytes say what it is; over fileMax is
// refused (the reader is read up to one byte past it).
func (h *Hub) SaveFile(session, name string, body io.Reader) (*SavedFile, error) {
	if h.sessionByIDLocked(session) == nil {
		return nil, ErrSessionOffline
	}
	dir, cwd, err := h.filesDir(session)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(body, fileMax+1))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, ErrFileTooBig
		}
		return nil, err
	}
	if len(data) > fileMax {
		return nil, ErrFileTooBig
	}
	ext := imageExt[strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))]
	if ext == "" || len(data) == 0 {
		return nil, ErrFileNotImage
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	_ = gitExclude(cwd) // (.symphony/ out of the repository's eye, when it is one; a plain folder has no exclude file)
	id := newID() + newID() + "." + ext
	path := filepath.Join(dir, id)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return &SavedFile{ID: id, Name: cutRunes(strings.TrimSpace(name), 120), URL: "/api/files/" + session + "/" + id, Size: int64(len(data)), Path: path}, nil
}

// FilePath is where a session's file is on disk, by the id it was served under; one that isn't there, or an id that
// isn't one the bridge makes, is ErrFileUnknown.
func (h *Hub) FilePath(session, id string) (string, error) {
	if !fileID.MatchString(id) {
		return "", ErrFileUnknown
	}
	dir, _, err := h.filesDir(session)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, id)
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return "", ErrFileUnknown
	}
	return path, nil
}

// fileType is the content type a served file goes out as, from its extension (the bytes were sniffed when it came).
func fileType(id string) string {
	for typ, ext := range imageExt {
		if strings.HasSuffix(id, "."+ext) {
			return typ
		}
	}
	return "application/octet-stream"
}
