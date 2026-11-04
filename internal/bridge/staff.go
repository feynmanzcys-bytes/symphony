package bridge

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The employees page (Settings → Employees → "All employees", ui/staff.js): what each employee starts with from the
// page, changed for one or for several at once, or imported from a file, which also makes new employees. What one
// starts with is its account's Last, which the launch dialog fills in for "Start work" (and which a start from the page
// replaces with what it was started with). Names stay as they are: a name is how the sessions find each other
// (SendMessage), and the slug of its task branches. An advisor belongs to its task and isn't on the page.

// maxImport is how many employees one import may name.
const maxImport = 100

// StaffEntry is one employee of an import (POST /api/accounts/import): its name, and what it starts with from the page.
// A field left out stays as it is (a new employee's, as the settings say); Dir "" is the employee's own folder
// (botFolder), a value of the drop-downs "" Claude Code's own settings.
type StaffEntry struct {
	Name           string  `json:"name"`
	Dir            *string `json:"dir,omitempty"`
	Model          *string `json:"model,omitempty"`
	Effort         *string `json:"effort,omitempty"`
	PermissionMode *string `json:"permissionMode,omitempty"`
	Context        *bool   `json:"context,omitempty"`
	Look           *Look   `json:"look,omitempty"` // the face (Account.Look): set as given; left out, as it is
}

// StaffImport is what an import did: the employees it made and the ones whose choices it changed (one it named that
// stays as it was is in neither), as the snapshot shows them (no tokens: a session started by hand gets its token from
// POST /api/accounts).
type StaffImport struct {
	Created []AccountView `json:"created"`
	Updated []AccountView `json:"updated"`
}

var (
	errImportEmpty     = newError("import_empty", "the import names no employee")
	errImportTooMany   = newError("import_too_many", fmt.Sprintf("an import names at most %d employees", maxImport), "max", strconv.Itoa(maxImport))
	errImportDuplicate = newError("import_duplicate", "the import names this employee twice")
)

// choices says whether the entry says anything of what the employee starts with.
func (e StaffEntry) choices() bool {
	return e.Dir != nil || e.Model != nil || e.Effort != nil || e.PermissionMode != nil || e.Context != nil
}

// over is what an employee starts with once the entry is applied: its choices with the entry's over them. One that has
// none yet starts from what the launch dialog would fill in for it: the settings' choices, and the folder it last worked
// in (dir, "" for its own). The result is new: snapshots made before may hold the old one.
func (e StaffEntry) over(last *LaunchRequest, d LaunchDefaults, lastDir, dir string) *LaunchRequest {
	r := LaunchRequest{Dir: lastDir, Model: d.Model, Effort: d.Effort, PermissionMode: d.PermissionMode, Context: d.Context}
	if last != nil {
		r = *last
	}
	if e.Dir != nil {
		r.Dir = dir
	}
	for _, f := range []struct{ to, from *string }{{&r.Model, e.Model}, {&r.Effort, e.Effort}, {&r.PermissionMode, e.PermissionMode}} {
		if f.from != nil {
			*f.to = *f.from
		}
	}
	if e.Context != nil {
		r.Context = *e.Context
	}
	return &r
}

// entryError says which entry of an import is wrong: its place in the list (from 1) and its name in the params, beside
// the error's own code and params, so the page can say which one to fix and why.
func entryError(err error, n int, name string) *Error {
	e := &Error{Code: "import_bad_entry", Text: err.Error(), Params: map[string]string{}}
	if ce := (*Error)(nil); errors.As(err, &ce) {
		e.Code = ce.Code
		for k, v := range ce.Params {
			e.Params[k] = v
		}
	}
	e.Params["entry"], e.Params["name"] = strconv.Itoa(n), name
	e.Text = fmt.Sprintf("employee %d (%s): %s", n, name, e.Text)
	return e
}

// ImportStaff makes and changes employees in one go. Each entry's name finds the employee to change (what it starts
// with from the page; nothing when the entry says nothing new of it), or names a new one, which takes the lowest free seat
// and, when the entry says nothing of what it starts with, goes by the settings until its first start. Everything is
// checked first, so either all of it is done or none: a name that can't be one, a name twice, an advisor's name, a
// folder that isn't an absolute one that's there, a choice the launch dialog doesn't offer. accounts.json is written
// once.
func (h *Hub) ImportStaff(entries []StaffEntry) (StaffImport, error) {
	if len(entries) == 0 {
		return StaffImport{}, errImportEmpty
	}
	if len(entries) > maxImport {
		return StaffImport{}, errImportTooMany
	}
	type entry struct {
		StaffEntry
		dir string // the folder checked, "" for its own
	}
	list := make([]entry, len(entries))
	seen := map[string]bool{}
	for i, e := range entries {
		e.Name = strings.TrimSpace(e.Name)
		if !validName(e.Name) {
			return StaffImport{}, entryError(ErrBadName, i+1, e.Name)
		}
		if seen[e.Name] {
			return StaffImport{}, entryError(errImportDuplicate, i+1, e.Name)
		}
		seen[e.Name] = true
		list[i].StaffEntry = e
		if e.Dir != nil && strings.TrimSpace(*e.Dir) != "" { // (os.Stat, outside the hub's lock)
			dir, err := normalizeDir(*e.Dir)
			if err != nil {
				return StaffImport{}, entryError(err, i+1, e.Name)
			}
			list[i].dir = dir
		}
		for _, c := range []struct {
			opts []Option
			v    *string
		}{{launchModels, e.Model}, {launchEfforts, e.Effort}, {launchPermissionModes, e.PermissionMode}} {
			if c.v != nil && !validOption(c.opts, *c.v) {
				return StaffImport{}, entryError(errBadOption, i+1, e.Name)
			}
		}
		if e.Look != nil && !e.Look.valid() {
			return StaffImport{}, entryError(ErrBadLook, i+1, e.Name)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	byName, taken := map[string]*Account{}, map[int]bool{}
	for _, a := range h.accounts {
		byName[a.Name], taken[a.Seat] = a, true
	}
	for i, e := range list {
		if a := byName[e.Name]; a != nil && a.Advisor {
			return StaffImport{}, entryError(ErrNameTaken, i+1, e.Name)
		}
	}
	// made and changed, remembered so that all of it can be taken back when accounts.json can't be written
	var made, changed []*Account
	was := map[*Account]*LaunchRequest{}
	wasLook := map[*Account]*Look{}
	defaults := h.settings.launchDefaults()
	seat := 0
	for _, e := range list {
		a := byName[e.Name]
		if a == nil {
			for taken[seat] {
				seat++
			}
			taken[seat] = true
			a = &Account{ID: newID(), Name: e.Name, Token: newToken(), Seat: seat, Created: h.now(), Look: e.Look}
			if e.choices() {
				a.Last = e.over(nil, defaults, "", e.dir)
			}
			h.accounts[a.ID] = a
			made = append(made, a)
			continue
		}
		// the face: set when the entry gives one that differs (the same one again changes nothing)
		newLook := e.Look != nil && (a.Look == nil || *a.Look != *e.Look)
		if newLook {
			wasLook[a] = a.Look
			a.Look = e.Look
			changed = append(changed, a)
		}
		if !e.choices() {
			continue
		}
		next := e.over(a.Last, defaults, a.Dir, e.dir)
		if a.Last != nil && *next == *a.Last {
			continue // (one going by the settings stops going by them: a change, whatever the values)
		}
		was[a] = a.Last
		a.Last = next
		if !newLook {
			changed = append(changed, a)
		}
	}
	if len(made) == 0 && len(changed) == 0 {
		return StaffImport{Created: []AccountView{}, Updated: []AccountView{}}, nil // (nothing to write, nothing to tell the pages)
	}
	if err := h.saveLocked(); err != nil {
		for _, a := range made {
			delete(h.accounts, a.ID)
		}
		for a, last := range was {
			a.Last = last
		}
		for a, look := range wasLook {
			a.Look = look
		}
		return StaffImport{}, errAccountsWrite
	}
	out := StaffImport{Created: h.viewsLocked(made), Updated: h.viewsLocked(changed)}
	h.broadcastLocked(nil)
	return out, nil
}

// viewsLocked shows accounts as the snapshot does, by seat.
func (h *Hub) viewsLocked(list []*Account) []AccountView {
	out := make([]AccountView, 0, len(list))
	for _, a := range list {
		out = append(out, h.accountViewLocked(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seat < out[j].Seat })
	return out
}
