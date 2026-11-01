# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | **한국어** | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Claude Code 세션들을 한 팀으로 꾸려 픽셀 아트 사무실에 앉혀 보세요. 누가 일하고 있고 누가 여러분을 기다리는지 한눈에 보이고, 직원마다 자기 창에서 대화할 수 있으며, 권한 승인과 AskUserQuestion 질문도 거기서 클릭 한 번으로 처리할 수 있습니다.

![사무실](docs/office.png)

## 설치

가장 빠른 방법: Claude Code를 열고 아래 문장을 붙여 넣으세요.

> https://github.com/feynmanzcys-bytes/symphony 설치해 줘

Claude Code가 이 README를 읽고 다음 절의 내용을 모두 해 줍니다. 직접 설치하려면 같은 단계를 따라 하세요.

### AI 에이전트용: 설치 런북

사용자가 symphony 설치를 부탁했기 때문에 이 글을 읽고 있을 것입니다. 아래 단계를 셸에서 순서대로 실행하고, 마지막에 6단계의 보고를 하세요. 단계에서 따로 말하지 않는 한 사용자에게 아무것도 묻지 마세요. 일상적인 선택은 직접 하고, 보고에 적어 두면 됩니다.

symphony는 두 부분으로 되어 있고, 둘 다 있어야 합니다.

1. **`symphony` 프로그램**(bridge): 바이너리 하나로 된 로컬 웹 서버로, `http://127.0.0.1:7788/`에서 사무실 페이지를 제공하고 세션을 실행합니다. 사용자의 `PATH`에 `symphony`라는 이름으로 있어야 합니다. 세션이 실행 중인 bridge를 찾지 못하면 mod가 `symphony -background`로 bridge를 시작하기 때문입니다.
2. **`symphony-bridge` 플러그인**(mod): 이 저장소의 마켓플레이스에 있는 Claude Code 플러그인으로, 사용자 범위로 설치합니다. `SYMPHONY_TOKEN`을 가지고 시작한 세션에서만 동작하고, 다른 세션에서는 아무것도 하지 않습니다.

#### 1단계: 컴퓨터 확인

```bash
uname -sm                 # Linux/macOS: 아래에서 고를 압축 파일이 정해집니다 (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # Claude Code 2.1.x 필요 (2.1.316에서 테스트)
git --version             # 협업 작업에만 필요. 없으면 보고에 적되 멈추지는 마세요
symphony -version         # 이미 설치돼 있나? "symphony v0.8.0 (darwin/arm64)" 출력
curl -s http://127.0.0.1:7788/api/state | head -c 40   # bridge가 이미 실행 중인가? {"type":"snapshot",... 출력
claude plugin list --json # mod가 이미 설치돼 있나? "id":"symphony-bridge@symphony"를 찾으세요
```

Windows에서는 PowerShell을 쓰세요. `$env:PROCESSOR_ARCHITECTURE`는 `AMD64` 또는 `ARM64`이고, curl 대신 `Invoke-WebRequest http://127.0.0.1:7788/api/state`를 씁니다.

`claude`가 없으면 멈추고, 먼저 Claude Code를 설치하라고 사용자에게 알리세요. 프로그램과 mod가 둘 다 이미 있으면 대신 아래 "업데이트"로 가세요.

#### 2단계: 프로그램 받기

되는 방법 중 첫 번째를 쓰세요.

- **A. `gh`로** (이 저장소가 비공개인 동안에도 됩니다. `gh auth status`가 로그인 상태이기만 하면 됩니다):
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. `curl`로** (저장소가 공개된 뒤에만 됩니다. 404가 나오면 A나 C를 쓰세요):
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. 소스에서** (Go 1.27 이상. 이 클론은 4단계에서 다시 씁니다):
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  바이너리를 빌드하세요. `-autostart`가 `go run` 빌드를 거부하므로 `go run`으로는 안 됩니다.

| 시스템 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple 실리콘) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

압축 파일마다 프로그램(`symphony`, Windows에서는 `symphony.exe`), `LICENSE`, `README.md`가 들어 있습니다. 릴리스의 `checksums.txt`에는 모든 압축 파일의 SHA-256이 있습니다(`sha256sum -c --ignore-missing checksums.txt`, macOS에서는 `shasum -a 256 -c`).

#### 3단계: PATH에 넣기

사용자의 `PATH`에 이미 들어 있고 `sudo` 없이 쓸 수 있는 폴더를 고르세요. `~/.local/bin`, `/usr/local/bin`, `/opt/homebrew/bin` 순서로 찾습니다. 하나도 없으면 `~/.local/bin`을 만들어 셸의 rc 파일(`~/.zshrc` 또는 `~/.bashrc`)에 추가하고, 보고에 그렇게 적으세요.

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: 바이너리에 서명이 없습니다
symphony -version          # "symphony v0.8.0 (linux/amd64)" 출력: 버전과 플랫폼
```

Windows에서는 `Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`를 실행한 다음, 그 폴더를 사용자의 `Path`에 추가하고(`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`) 새 셸을 여세요. 처음 실행할 때 SmartScreen이 경고할 수 있습니다. "추가 정보"를 누른 뒤 "실행"을 누르세요.

이렇게 한 뒤에도 지금 쓰는 셸이 `symphony`를 찾지 못하면(바꾸기 전에 `PATH`를 읽어 두었기 때문입니다), 나머지 단계에서는 절대 경로를 쓰세요.

#### 4단계: mod 설치

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

이 명령은 저장소를 플러그인 마켓플레이스 `symphony`로 추가하고, 아무것도 묻지 않고 mod를 사용자 범위로 설치합니다. 저장소를 git으로 클론하므로, 저장소가 비공개인 동안에는 GitHub에 git으로 접근할 수 있어야 합니다. 그 때문에 실패하면 클론을 마켓플레이스로 쓰세요(2단계 C의 클론, 없으면 새로 클론): `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. 클론을 마켓플레이스로 쓰면 세션은 그 폴더에서 mod를 읽고, 거기서 `git pull`하면 업데이트됩니다.

확인:

```bash
claude plugin list --json   # 항목 하나에 "id":"symphony-bridge@symphony", "scope":"user", "enabled":true가 있어야 합니다
```

- `"scope":"project"` 또는 `"local"`: mod가 한 폴더에서만 로드됩니다. `claude plugin uninstall symphony-bridge@symphony`를 실행한 뒤 `--scope user`로 다시 설치하세요.
- `"enabled":false`: `claude plugin enable symphony-bridge@symphony`.

#### 5단계: bridge 시작

```bash
symphony -autostart on -open=false
```

이 명령은 bridge를 지금 시작하고, 앞으로 로그인할 때마다 시작하게 합니다(Linux에서는 systemd 사용자 서비스, macOS에서는 launchd 에이전트, Windows에서는 레지스트리 Run 키). 그리고 다음을 출력합니다.

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

사용자가 로그인할 때 시작되는 것을 원하지 않는다고 했다면, 대신 `symphony -background -open=false`를 실행하세요. bridge를 터미널과 분리해 시작하고 `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log`를 출력합니다. bridge가 이미 실행 중이면 두 명령 모두 `a symphony bridge is already running at http://127.0.0.1:7788/`라고 답합니다. 문제없는 상황이니 또 시작하지 마세요.

요청 하나로 전체 연결을 확인합니다.

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

응답에 `"enabled":true`와 `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.8.0"}`가 있으면, bridge가 실행 중이고 `claude`를 찾았으며 세션이 로드할 위치에 mod가 설치된 것입니다. `"enabled":false`이면 `mod.code`가 무엇이 문제인지, `mod.fix`가 그것을 고치는 명령을 알려 줍니다. `mod_not_installed`나 `mod_project_scope`(4단계로 돌아가기), `mod_disabled`(활성화하기), `mod_check_failed`(`claude plugin list --json`이 실패한 것이니 Claude Code가 너무 오래된 버전일 가능성이 큽니다)입니다. `"mod":null`은 bridge가 자기 `PATH`에서 `claude`를 아예 찾지 못했다는 뜻입니다(로그에 나옵니다). claude를 거기에 넣거나 `-claude /path/to/claude`로 bridge를 시작하세요.

bridge의 로그는 데이터 폴더(Linux는 `~/.config/symphony`, macOS는 `~/Library/Application Support/symphony`, Windows는 `%AppData%\symphony`)의 `bridge.log`입니다. 로그인할 때 시작된 bridge는 Linux에서는 `journalctl --user -u symphony`, macOS에서는 `~/Library/Logs/symphony.log`에 로그를 남깁니다.

#### 6단계: 사용자에게 보고

브라우저에서 사무실을 열고(macOS는 `open http://127.0.0.1:7788/`, Linux는 `xdg-open`, Windows는 `start`) 사용자에게 알려 주세요.

- 사무실은 `http://127.0.0.1:7788/`에 있습니다. 빈자리의 "+"를 눌러 첫 직원을 뽑으세요. 폴더를 고르면 그 폴더에서 bridge 안에 Claude Code 세션이 시작되고, 페이지에 있는 그 직원의 창이 곧 그 세션의 채팅입니다.
- 사무실에는 페이지에서 시작한 세션(또는 `SYMPHONY_TOKEN`으로 직접 시작한 세션, 아래 "세션 시작" 참고)만 나옵니다. 이 설치를 하고 있는 세션을 포함해 사용자의 다른 Claude Code 세션은 보이지 않습니다.
- 무엇을 어디에 설치했는지: 프로그램 경로, 사용자 범위의 mod, bridge를 로그인 시 시작으로 했는지 백그라운드로 시작했는지, `PATH`를 바꿨다면 그 내용, 그리고 `git`이 없다면 없다는 사실.
- 모두 되돌리는 방법: 아래 "제거".

규칙: bridge를 이 컴퓨터 외의 주소에 바인딩하지 마세요(`-addr`는 `127.0.0.1:7788` 그대로 둡니다. 페이지에는 로그인이 없습니다). `sudo`로 실행하지 말고, 같은 주소에 두 번째 bridge를 시작하지 마세요.

#### 업데이트

2단계처럼 새 릴리스를 내려받아 3단계처럼 프로그램을 바꿔 넣으세요. mod는 `claude plugin update symphony-bridge@symphony`로 따라옵니다(클론을 마켓플레이스로 쓴다면 대신 거기서 `git pull`). 실행 중인 bridge는 다시 시작할 때까지 예전 프로그램으로 돌아갑니다. `symphony -autostart off` 다음에 `symphony -autostart on -open=false`를 실행하세요(또는 시작했던 방식대로 멈췄다가 다시 시작하세요). bridge를 멈추면 bridge가 시작한 세션도 끝나므로 먼저 사용자에게 알리세요. 프로그램과 mod는 같은 버전 번호를 씁니다. 릴리스마다 바뀐 점은 [CHANGELOG.md](CHANGELOG.md)에 있습니다.

#### 제거

```bash
symphony -autostart off                           # bridge와 그 세션을 멈추고, 로그인할 때 더는 시작하지 않습니다
claude plugin uninstall symphony-bridge@symphony  # mod
claude plugin marketplace remove symphony         # 마켓플레이스 항목
rm ~/.local/bin/symphony                          # 프로그램 (3단계에서 넣은 위치)
```

데이터 폴더(token이 든 계정, 메시지 기록, 작업)는 남습니다. 전부 지우려면 이 폴더도 지우세요.

## 주요 기능

- **세션 하나가 채팅 하나**: 직원을 누르면 페이지 가운데에 그 직원의 창이 열립니다. 위에는 직원 정보, 아래에는 세션 전체가 채팅으로: 입력한 프롬프트, 모델의 생각(접힌 상태, 진행 중에는 주황색 “생각 중…”), 흘러나오는 답변, 도구 호출, 다른 세션에서 온 메시지(보라색). 아래 입력란에 쓰면 그 세션의 프롬프트가 되고, “중지” 버튼으로 끊을 수 있습니다.
- **브라우저에서 승인**: 권한 승인(명령 실행, 파일 수정)과 AskUserQuestion 질문은 그 직원 창의 입력란 위로 카드처럼 올라오고, 상단 바의 빨간 숫자를 누르면 바로 갈 수 있습니다. bridge가 꺼져 있거나 계정을 알아보지 못하면 세션은 평소처럼 터미널에서 처리하므로, 멈춰 버리는 일은 없습니다.
- **픽셀 아트 사무실**: 세션 하나하나가 정해진 자리에 앉은 직원입니다. 초록은 쉬는 중, 노랑은 작업 중, 빨강은 여러분의 승인이나 답변을 기다리는 중, 회색은 퇴근입니다. 쉬는 직원은 처음엔 자리에서 휴대폰을 보거나 게임을 하다가 곧 사무실을 돌아다닙니다. 휴게실 소파에서 TV 보기, 테이블 축구, 술래잡기, 원탁에서 수다, 커피, 창밖 구경을 하고, 한참 지나면 낮잠을 자러 가는데 주로 회의실에 숨어서 잡니다. 캐릭터의 덩치는 모델을, 근육은 사고 강도를 따라갑니다. 화면은 큰 회사 건물의 한 층으로, 창을 가득 채웁니다. 가운데에 업무 공간, 왼쪽에 명패가 달린 3~5인용 작은 회의실, 오른쪽에 회의실과 휴게실이 있고, 직원들은 엘리베이터로 출퇴근합니다. 위쪽 바에서 픽셀 크기도 4단계로 고를 수 있습니다. 사무실은 그대로이고, 단계가 올라갈수록 더 작은 픽셀과 더 많은 디테일로 그려집니다.
- **찾아서 바로 불러오기**: 쉬는 직원을 클릭하면 자기 자리로 돌아가 다음 일이 올 때까지 거기 있습니다. 위쪽 바의 검색창에서 이름, 세션 이름, 폴더로 직원을 찾을 수 있습니다(`/` 키로 바로 이동). 그 옆의 상태 숫자(쉬는 중, 작업 중, 대기 중, 퇴근)를 클릭하면 그 상태의 직원이 모두 강조됩니다. 바 아래의 손잡이로 바를 위로 접었다가 다시 펼 수 있습니다.
- **세션 간 메시지**: 세션끼리 메시지를 보내면(SendMessage) 편지 봉투가 사무실을 가로질러 날아가고, 메시지 기록에 남습니다.
- **페이지에서 세션 시작, 창은 뜨지 않음**: 빈자리의 "+"를 눌러 새 직원을 뽑고 폴더, 모델, 사고 강도, 권한 모드를 고르세요(새 직원은 모두 허용, Opus, 최대 사고 강도가 기본). 퇴근한 직원의 창에서 "출근"을 누르면 원래 자리로 돌아옵니다. 세션은 bridge 안의 전용 터미널에서 돌아가므로 창이 뜨지도, 포커스를 빼앗기지도 않습니다. 보고 싶을 때는 직원 창의 "터미널 열기"로 그 터미널을 볼 수 있고, 그 창을 닫아도 아무것도 바뀌지 않습니다.
- **회의실에서 팀으로 일하기**: 비어 있는 작은 회의실을 누르고 작업을 적은 뒤 멤버를 2~5명 고릅니다. 멤버는 직원(각자 이 작업만 하는 두 번째 세션을 받습니다)이나 이번 한 번만 함께하는 어드바이저입니다. 기본으로는 첫 번째 멤버가 리더가 되어 SendMessage로 일을 나눠 줍니다. 멤버마다 자기 git worktree의 자기 브랜치에서 일하므로 서로의 파일을 건드릴 일이 없습니다. 작업이 끝날 때까지 멤버들은 회의실에 앉아 있습니다.
- **10개 언어**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil), Русский를 지원합니다. 페이지는 브라우저 언어를 따르고 위쪽 바에서 언어를 바꿀 수 있습니다. bridge 메시지는 `-lang`을, 없으면 시스템 언어(`LANG` 등)를, 그것도 없으면 영어를 따릅니다. mod 메시지는 `SYMPHONY_LANG`(페이지에서 시작한 세션이면 그 페이지의 언어)을, 없으면 시스템 언어를, 그것도 없으면 영어를 따릅니다.
- **로컬 전용**: bridge는 Go 프로그램 하나(표준 라이브러리만 사용하고, 엔드투엔드 테스트에만 chromedp를 씁니다)이고 페이지는 순수 JavaScript이며, 외부 서비스에 전혀 의존하지 않습니다.

![브라우저에서 승인](docs/approve.png)

## 요구 사항

- Claude Code 2.1.316 (이 버전에서 테스트했습니다). mod는 Claude Code의 function hooks를 쓰는데, 아직 얼리 액세스 단계의 API이므로 Claude Code를 업그레이드한 뒤에는 엔드투엔드 테스트를 돌려 보세요.
- bridge는 Linux, macOS, Windows용으로 미리 빌드된 프로그램이 있습니다. Go 1.27 이상은 소스에서 빌드할 때만 필요합니다.
- `git`: 협업 작업에만 필요합니다. 작업의 멤버는 저마다 git worktree에서 일합니다.
- 그 외에는 아무것도 필요 없습니다. 페이지에서 시작한 세션은 모든 플랫폼에서 bridge 안에서 돌아갑니다.

## bridge 실행

`symphony`는 bridge를 시작하고 브라우저에서 사무실(<http://127.0.0.1:7788/>)을 엽니다(`-open=false`로 끌 수 있습니다). 승인만 처리하는 더 간단한 페이지는 <http://127.0.0.1:7788/approve>에 있습니다. bridge가 실행 중일 때 다시 시작하면 사무실만 엽니다. Windows에서 `symphony.exe`를 더블클릭하면 별도의 창에서 실행되며, 그 창을 닫으면 bridge도 멈춥니다.

| 플래그 | 기본값 | 설명 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 수신 대기할 주소. 페이지에 아직 로그인 기능이 없으니 이 컴퓨터 외의 주소에는 바인딩하지 마세요 |
| `-data` | `$SYMPHONY_DATA`, 없으면 설정 폴더 안의 `symphony` | 데이터 폴더: token이 든 계정(`accounts.json`), 메시지 기록(`messages.json`), 작업(`tasks.json`), `-background`의 로그(`bridge.log`) |
| `-claude` | `claude` | claude 실행 파일 |
| `-lang` | 시스템 언어, 없으면 `en` | bridge 메시지 언어(`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES`, `LANG`으로 기본값이 정해집니다) |
| `-mod` | 비어 있음 | 개발용: 이 폴더의 mod를 `--plugin-dir`로 세션에 넘겨, 설치된 사본 대신 쓰게 합니다 |
| `-web` | 비어 있음 | 개발용: 이 폴더의 페이지를 제공해서, 수정한 내용이 새로고침하면 바로 보입니다 |
| `-open` | `true` | bridge가 시작되면 브라우저에서 사무실을 엽니다. `-addr`에서 이미 bridge가 실행 중이면 그 사무실만 엽니다 |
| `-autostart` | — | `on`: 로그인할 때마다 bridge를 자동으로 시작하고(함께 준 플래그 포함) 지금 바로 시작합니다. `off`: 되돌립니다 |
| `-background` | — | bridge를 백그라운드에서 시작합니다. 터미널과 분리되고 로그는 데이터 폴더의 `bridge.log`에 남기며, 응답하면 돌아옵니다 |
| `-version` | — | 버전을 출력하고 종료합니다 |

**로그인할 때 시작**: `symphony -autostart on`을 실행하면 이후 로그인할 때마다 bridge가 자동으로 시작되고(함께 준 플래그, 예를 들어 `-addr`도 그대로 씁니다) 지금 바로 시작됩니다. `symphony -autostart off`로 되돌릴 수 있습니다. Linux에서는 systemd 사용자 서비스(로그는 `journalctl --user -u symphony`), macOS에서는 launchd 에이전트(로그는 `~/Library/Logs/symphony.log`), Windows에서는 레지스트리 Run 키의 값입니다(bridge는 콘솔 창에서 실행되며, 창을 닫으면 다음 로그인 때까지 멈춥니다). `go run`이 아니라 symphony 프로그램 자체가 필요합니다.

**소스에서** 실행하려면 Go 1.27 이상이 필요합니다: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## 세션 시작

**페이지에서**: 빈자리의 "+"를 누르고 이름을 입력한 뒤 폴더와 옵션을 고릅니다. bridge는 자기 안의 전용 터미널에서 `claude`를 실행하며 창은 열리지 않습니다. 직원 창에 세션이 채팅으로 보이고, "터미널 열기" 버튼은 그 세션에 연결된 터미널 창을 엽니다(아무 터미널에서나 `symphony attach <id>`로도 됩니다. `Ctrl-]`나 창 닫기로 분리되며 세션은 계속됩니다). 세션은 bridge의 자식 프로세스이므로 bridge를 멈추면 세션도 끝납니다.

**터미널에서**: 계정을 만들어 token을 받은 다음, 그 token으로 `claude`를 시작합니다.

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 응답에 token이 들어 있습니다
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

권한 모드는 `manual`을 추천합니다. `auto` 모드에서는 대부분의 작업을 아무에게도 묻지 않기 때문에 페이지까지 오는 승인이 거의 없습니다. bridge가 기본 주소에 있지 않다면 `SYMPHONY_BRIDGE=http://host:port`도 설정하세요. mod 메시지의 언어는 `SYMPHONY_LANG`으로 고릅니다(없으면 시스템 언어, 그것도 없으면 영어).

이 컴퓨터의 bridge 주소에서 아무 응답이 없으면, 세션이 `symphony -background -open=false -addr <that address>`로 bridge를 직접 시작합니다(`symphony`가 PATH에 있을 때). bridge는 세션이 끝나도 계속 실행됩니다. 계정이 기본 데이터 폴더에 없다면 `SYMPHONY_DATA`를 설정하세요.

## 협업 작업

작은 회의실은 세션 2~5개가 한 팀이 되어 작업 하나를 함께 하는 곳입니다. 작업에는 `git`과 0.7.0 버전의 mod가 필요합니다(`claude plugin update symphony-bridge@symphony`).

**작업 시작**: 비어 있는 작은 회의실을 누르고 할 일을 적습니다. 원하는 순서대로 멤버를 체크하고(2명부터 회의실 좌석 수까지), 멤버마다 모델과 사고 강도를, 팀 전체의 권한 모드(기본은 모두 허용)를 고른 뒤 "작업 시작"을 누릅니다. 멤버는 둘 중 하나입니다.

- **직원**: 이 작업만 하는 두 번째 세션을 받아 그 직원의 폴더에서 일합니다. 자리에 있는 원래 세션은 그대로입니다. 직원은 한 번에 작업 하나에만 들어갈 수 있고, 한 번도 일한 적 없는 직원은 아직 폴더가 없으니 먼저 "출근"을 한 번 누르세요.
- **어드바이저**: 이 작업을 위해 늘 쓰던 항목(이름, 폴더, 모델, 사고 강도)으로 만드는 일회성 멤버입니다. 자리가 없고, 엘리베이터로 올라왔다가 작업이 끝나면 사라집니다.

**리더**: 기본으로는 첫 번째 멤버가 리더입니다. 작업을 나누고, SendMessage로 각자 맡을 부분을 나눠 주고, 결과를 검토하고, 다른 멤버의 브랜치를 자기 브랜치에 병합한 뒤 무엇을 했는지 알려 줍니다. 리더 체크를 풀면 모두가 작업 전체를 받고, 멤버끼리 나눠 맡습니다. 어느 쪽이든 각 멤버의 첫 프롬프트는 bridge가 보냅니다. 거기에는 팀에 누가 있는지(멤버끼리는 SendMessage로 이름을 대고 연락합니다), 모두의 브랜치, 그리고 규칙이 들어 있습니다.

**git**: 멤버마다 자기 폴더의 전용 git worktree에서 자기 브랜치 `symphony/<task id>/<member>`(예: `symphony/T1/bob`. 같은 이름의 멤버가 또 있으면 `-2`가 붙습니다) 위에서 일하므로, 서로의 파일을 건드릴 일이 없고 여러분의 체크아웃도 그대로입니다. worktree는 폴더 안의 `.symphony/worktrees/`에 있고, `.git/info/exclude`로 git에서 숨깁니다(여러분의 `.gitignore`는 건드리지 않습니다). 작업이 끝나면 멤버가 커밋하지 않은 변경은 그 멤버의 브랜치에 커밋되므로 잃어버리는 것이 없고, worktree는 지워집니다.

- 여러분의 저장소에서는 아무것도 병합하지 않습니다. 브랜치가 남아 있으니 직접 병합하세요. 리더가 있다면 리더의 브랜치에 모두의 결과가 모여 있습니다.
- git 저장소가 아닌 폴더는 작업을 위해 저장소가 되고(`git init`, 그리고 안의 모든 것을 한 번 커밋), 그 뒤에도 저장소로 남습니다. 작업이 끝나면 멤버의 브랜치가 거기에 병합되어 폴더에 결과가 담깁니다. `.git`과 브랜치는 그대로 남습니다.
- 병합에 실패하면 그 병합은 되돌려져 기준 브랜치는 병합 전 그대로이고, 브랜치는 작업 내용과 함께 남으며, 어느 브랜치가 충돌했는지 작업 창에 보여 줍니다.

**사무실에서는**: 작업하는 동안 멤버들은 회의실 탁자에 앉아 있고, 직원의 자리에는 빈 의자만 남습니다. 회의실 명패에는 작업 ID가 나옵니다. 명패의 불빛은 비어 있을 때는 초록, 작업 중에는 노랑, 멤버가 여러분의 승인이나 답변을 기다릴 때는 빨강입니다. 회의실을 누르면 작업 창이 열립니다. 작업 내용, 멤버와 각자의 폴더와 브랜치, 팀 채팅, 그리고 "작업 끝내기"가 있습니다. 회의실에 있는 멤버를 눌러도 작업 창이 열립니다. 작업 창에서 멤버를 누르면 그 멤버의 채팅 창이 작업 창 옆에 붙어서 열리는데, 직원 창과 똑같고 한 번에 한 명만 열립니다(다른 멤버를 누르면 바뀝니다). 상단 바의 빨간 숫자나 검색창으로 작업 멤버를 열 때도 마찬가지입니다.

**팀 채팅**: 작업 창에는 멤버끼리 주고받은 메시지가 나옵니다. 메시지마다 보낸 멤버의 픽셀 캐릭터와 이름이 붙고, 본문은 `@`와 받는 사람으로 시작합니다. 아래 입력란에서 여러분도 팀에 직접 쓸 수 있습니다. `@bob-T1 …`은 그 멤버에게만 가고(`@names`를 여러 개 쓰면 그 모두에게 갑니다. `@`를 치면 멤버 목록이 열리고, 화살표 키와 Enter 또는 클릭으로 고릅니다), `@` 없는 메시지는 작업의 모든 멤버에게 갑니다. Enter로 보내고 Shift+Enter로 줄을 바꿉니다.

작업은 작업 창에서 끝내거나 마지막 세션이 끝나면 끝납니다. bridge는 멤버의 세션을 멈추고, 위에서 말한 대로 git 쪽을 정리하고, 어드바이저를 지웁니다. bridge가 멈출 때 아직 진행 중이던 작업은 다음에 bridge가 시작될 때 같은 방식으로 끝납니다.

## 계정과 자리

- symphony에서 신원은 이름, token, 고정된 자리로 이루어집니다. 계정 하나는 한 번에 세션 하나만 쓸 수 있고, 참여 중인 작업에서 하나를 더 쓸 수 있습니다. 그 밖의 세션은 거부되고, 그 세션의 승인은 터미널에 남습니다.
- Claude 로그인은 여기서 관리하지 않습니다. 모든 세션이 Claude 로그인 하나를 함께 써도 됩니다.
- 데이터는 `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS), `%AppData%\symphony` (Windows), 또는 `-data` / `SYMPHONY_DATA`로 지정한 곳에 저장됩니다. `accounts.json`에는 token이 들어 있어서 소유자만 읽을 수 있습니다(0600). Windows에서는 사용자 폴더의 권한이 이 파일을 보호합니다.

## 보안

- bridge는 이 컴퓨터를 향한 요청만 받고, 다른 웹사이트가 여러분의 브라우저를 통해 보내는 쓰기 요청은 거부합니다. 페이지에서 세션을 시작할 수 있다는 것은 곧 이 컴퓨터에서 명령을 실행할 수 있다는 뜻이기 때문입니다.
- 페이지에 아직 로그인 기능이 없으므로 bridge를 이 컴퓨터 외의 주소에 **바인딩하지 마세요**.
- 프로그램에는 서명이 없습니다. 브라우저로 내려받은 사본은 macOS가 개발자를 확인할 수 없다며 막을 수 있습니다. "시스템 설정 → 개인정보 보호 및 보안"에서 "그래도 열기"를 누르거나, `xattr -d com.apple.quarantine symphony`를 한 번 실행하세요. Windows에서도 SmartScreen이 경고할 수 있습니다. "추가 정보"를 누른 뒤 "실행"을 누르세요. 릴리스의 `checksums.txt`에는 모든 압축 파일의 SHA-256이 들어 있습니다.

## 플랫폼

| 시스템 | 페이지에서 시작한 세션 | "터미널 열기" | 테스트 범위 |
| --- | --- | --- | --- |
| Linux | bridge 안의 의사 터미널 | 그래픽 터미널(x-terminal-emulator, gnome-terminal, konsole, xterm)에서 `symphony attach` 실행 | 엔드투엔드 테스트 전체 |
| macOS | bridge 안의 의사 터미널 | Terminal.app에서 `symphony attach` 실행 | 엔드투엔드 테스트 전체 |
| Windows | bridge 안의 의사 콘솔(ConPTY) | 새 콘솔 창에서 `symphony attach` 실행 | 크로스 컴파일과 단위 테스트만 |

"이 폴더 신뢰"는 모든 플랫폼에서 bridge가 대신 눌러 줍니다.

## 개발

```bash
go test -race ./...                         # bridge와 번역 단위 테스트
claude plugin test mods/symphony-bridge     # mod 단위 테스트
claude plugin validate .                    # 마켓플레이스 검사
claude plugin validate mods/symphony-bridge  # mod 검사
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 엔드투엔드: 실제 세션을 띄우고 토큰을 소모합니다
```

- mod 작업: `go run ./cmd/bridge -mod mods/symphony-bridge`로 실행하면 페이지에서 시작한 세션이 `--plugin-dir`로 여러분의 mod 사본을 받습니다. 직접 시작하는 세션에는 `--plugin-dir mods/symphony-bridge`를 손수 붙이세요.
- 페이지 작업: `go run ./cmd/bridge -web web`으로 실행하면 새로고침만으로 수정 내용이 보입니다.
- 실제 세션 없이 사무실 보기: 두 번째 bridge를 띄워 가짜 활동으로 채웁니다. `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`를 실행한 다음 `go run ./cmd/demo -base http://127.0.0.1:7799`를 실행하세요.
- 번역: 언어마다 파일이 하나씩 있습니다. 페이지용은 `web/locales/<lang>.json`, bridge용은 `internal/i18n/locales/<lang>.json`이고 키는 `en.json`과 같습니다. mod의 몇 안 되는 메시지는 `mods/symphony-bridge/hooks/messages.ts`에 있습니다. 단위 테스트는 모든 언어에 모든 키와 같은 `{placeholders}`가 있는지 확인하고, 엔드투엔드 테스트 `TestI18n`은 모든 언어로 페이지를 열어 봅니다.
- 엔드투엔드 테스트에는 claude, tmux(테스트가 직접 움직이는 세션용), Chromium 또는 Chrome이 필요하므로 Linux와 macOS에서 돌아갑니다. 기본적으로 이 저장소의 mod를 쓰고, `SYMPHONY_E2E_MOD=installed`로 설정하면 설치된 mod를 씁니다. 스크린샷은 `e2e/artifacts/`에 저장됩니다.
- 릴리스: `mods/symphony-bridge/.claude-plugin/plugin.json`의 버전을 올리고, `CHANGELOG.md`에 `## vX.Y.Z (date)` 절을 추가하고, dev를 main에 반영한 다음, main의 그 커밋에 태그를 달아 push합니다(`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`). GitHub Actions는 태그가 `plugin.json`의 버전과 같은지, 그 절이 있는지 확인하고, 모든 플랫폼용 프로그램을 빌드해 그 절로 노트가 시작되는 릴리스를 게시합니다(`.github/workflows/release.yml`. `-rc.1` 같은 접미사를 붙이면 프리릴리스가 됩니다). `.github/release.sh v0.7.0 dist`로 같은 압축 파일을 로컬에서 만들 수 있습니다.

| 폴더 | 설명 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod (TypeScript) |
| `.claude-plugin/marketplace.json` | `claude plugin install`이 읽는 플러그인 마켓플레이스 |
| `cmd/bridge/`, `internal/bridge/` | bridge |
| `internal/i18n/` | 언어 감지와 bridge 번역 |
| `web/` | 사무실 페이지와 승인 페이지, 그리고 그 번역 (bridge에 내장됨) |
| `cmd/demo/` | 사무실에 보여 줄 가짜 활동 |
| `e2e/` | 엔드투엔드 테스트 |

## 라이선스

[MIT](LICENSE)
