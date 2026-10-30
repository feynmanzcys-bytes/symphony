# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | **한국어** | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Claude Code 세션들을 한 팀으로 꾸려 픽셀 아트 사무실에 앉혀 보세요. 누가 일하고 있고 누가 여러분을 기다리는지 한눈에 보이고, 권한 승인과 AskUserQuestion 질문은 웹 팝업에서 클릭 한 번으로 처리할 수 있습니다.

![사무실](docs/office.png)

## 주요 기능

- **브라우저에서 승인**: 권한 승인(명령 실행, 파일 수정)과 AskUserQuestion 질문이 웹 페이지로 넘어옵니다. bridge가 꺼져 있거나 계정을 알아보지 못하면 세션은 평소처럼 터미널에서 처리하므로, 멈춰 버리는 일은 없습니다.
- **픽셀 아트 사무실**: 세션 하나하나가 정해진 자리에 앉은 직원입니다. 초록은 쉬는 중, 노랑은 작업 중, 빨강은 여러분의 승인이나 답변을 기다리는 중, 회색은 퇴근입니다. 쉬는 직원은 처음엔 자리에서 휴대폰을 보거나 게임을 하다가 곧 사무실을 돌아다닙니다. 휴게실 소파에서 TV 보기, 테이블 축구, 술래잡기, 원탁에서 수다, 커피, 창밖 구경을 하고, 한참 지나면 낮잠을 자러 가는데 주로 회의실에 숨어서 잡니다. 캐릭터의 덩치는 모델을, 근육은 사고 강도를 따라갑니다. 화면은 큰 회사 건물의 한 층으로, 창을 가득 채웁니다. 가운데에 업무 공간, 왼쪽에 명패가 달린 3~5인용 작은 회의실, 오른쪽에 회의실과 휴게실이 있고, 직원들은 엘리베이터로 출퇴근합니다. 위쪽 바에서 픽셀 크기도 4단계로 고를 수 있습니다. 사무실은 그대로이고, 세밀하게 할수록 더 작은 픽셀과 더 많은 디테일로 그려집니다.
- **찾아서 바로 불러오기**: 쉬는 직원을 클릭하면 자기 자리로 돌아가 다음 일이 올 때까지 기다립니다. 위쪽 바의 검색창에서 이름이나 폴더로 직원을 찾을 수 있습니다(`/` 키로 바로 이동). 그 옆의 상태 칩(쉬는 중, 작업 중, 대기 중, 퇴근)을 클릭하면 그 상태의 직원이 모두 강조됩니다. 바 아래의 손잡이로 바를 위로 접었다가 다시 펼 수 있습니다.
- **세션 간 메시지**: 세션끼리 메시지를 보내면(SendMessage) 편지 봉투가 사무실을 가로질러 날아가고, 메시지 기록에 남습니다.
- **페이지에서 세션 시작**: 빈자리의 "+"를 눌러 새 직원을 뽑고 폴더, 모델, 사고 강도, 권한 모드를 고르세요. 퇴근한 직원의 카드에서 "출근"을 누르면 원래 자리로 돌아옵니다.
- **10개 언어**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil), Русский를 지원합니다. 페이지는 브라우저 언어를 따르고 위쪽 바에서 언어를 바꿀 수 있습니다. bridge 메시지는 시스템 언어(또는 `-lang`)를, mod 메시지는 그 세션을 시작한 페이지의 언어를 따릅니다.
- **로컬 전용**: bridge는 Go 프로그램 하나(표준 라이브러리만 사용)이고 페이지는 순수 JavaScript이며, 외부 서비스에 전혀 의존하지 않습니다.

![브라우저에서 승인](docs/approve.png)

## 요구 사항

- Claude Code 2.1.314 (이 버전에서 테스트했습니다). mod는 Claude Code의 function hooks를 쓰는데, 아직 얼리 액세스 단계의 API이므로 Claude Code를 업그레이드한 뒤에는 엔드투엔드 테스트를 돌려 보세요.
- bridge는 Linux, macOS, Windows용으로 미리 빌드된 프로그램이 있습니다. Go 1.27 이상은 소스에서 실행할 때만 필요합니다.
- 페이지에서 세션을 시작하려면: Linux와 macOS에서는 tmux가 가장 좋습니다.

## 설치

### 1. mod 설치

Claude Code에서 다음을 입력합니다:

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

`y`를 눌러 마켓플레이스를 추가한 다음 설치 범위를 고릅니다(기본값은 사용자 범위). 셸에서 설치할 수도 있습니다:

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

저장소를 클론했다면 클론한 저장소를 마켓플레이스로 쓸 수도 있습니다. 그러면 mod와 bridge가 같은 코드가 되고, `git pull` 후에 새로 여는 세션마다 새 버전이 적용됩니다:

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

사용자 범위로 설치한 mod는 모든 Claude Code 세션에서 로드되지만, `SYMPHONY_TOKEN`을 가지고 시작한 세션에서만 동작합니다. 다른 세션에서는 아무것도 하지 않습니다.

업데이트: `claude plugin update symphony-bridge@symphony`. 제거: `claude plugin uninstall symphony-bridge@symphony`.

### 2. bridge 실행

[최신 릴리스](https://github.com/feynmanzcys-bytes/symphony/releases/latest)에서 사용하는 시스템에 맞는 압축 파일을 내려받아 풀고, 안에 있는 `symphony`를 실행합니다.

| 시스템 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple 실리콘) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

예를 들어 Linux에서는:

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

Windows에서는 압축을 풀고 `symphony.exe`를 실행합니다. 더블클릭하면 별도의 창에서 실행되며, 그 창을 닫으면 bridge도 멈춥니다.

프로그램에 서명이 없어서 처음 실행할 때 막힐 수 있습니다. 브라우저로 내려받았다면 macOS가 "개발자를 확인할 수 없음"이라고 할 수 있습니다. "시스템 설정 → 개인정보 보호 및 보안"에서 "그래도 열기"를 누르거나, `xattr -d com.apple.quarantine symphony`를 한 번 실행하세요. Windows에서도 SmartScreen이 경고할 수 있습니다. "추가 정보"를 누른 뒤 "실행"을 누르세요. 릴리스의 `checksums.txt`에는 압축 파일마다 SHA-256이 들어 있습니다.

프로그램과 mod는 같은 버전 번호를 씁니다. 프로그램 버전은 `symphony -version`, mod 버전은 `claude plugin list`로 확인합니다. 업데이트하려면 새 릴리스를 내려받고 `claude plugin update symphony-bridge@symphony`를 실행하세요.

소스에서 실행할 수도 있습니다(Go 1.27 이상 필요).

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

bridge가 시작되면 브라우저에서 사무실(<http://127.0.0.1:7788/>)이 열립니다(`-open=false`로 끌 수 있습니다). 승인만 처리하는 더 간단한 페이지는 <http://127.0.0.1:7788/approve>에 있습니다. bridge가 이미 실행 중일 때 다시 시작하면 사무실만 엽니다.

| 플래그 | 기본값 | 설명 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 수신 대기할 주소. 페이지에 아직 로그인 기능이 없으니 이 컴퓨터 외의 주소에는 바인딩하지 마세요 |
| `-data` | 설정 폴더 안의 `symphony` | 계정(token 포함)과 메시지 기록을 저장하는 곳 |
| `-terminal` | `auto` | 페이지에서 시작한 세션에 쓸 터미널: `auto`, `tmux`, `window` 중 하나 |
| `-claude` | `claude` | claude 실행 파일 |
| `-lang` | 시스템 언어 | bridge 메시지 언어 |
| `-mod` | 비어 있음 | 개발용: 이 폴더의 mod를 `--plugin-dir`로 세션에 넘겨, 설치된 사본 대신 쓰게 합니다 |
| `-web` | 비어 있음 | 개발용: 이 폴더의 페이지를 제공해서, 수정한 내용이 새로고침하면 바로 보입니다 |
| `-open` | `true` | bridge가 시작되면 브라우저에서 사무실을 엽니다. `-addr`에서 이미 bridge가 실행 중이면 그 사무실만 엽니다 |
| `-version` | — | 버전을 출력하고 종료합니다 |

### 3. 세션 시작

**페이지에서**: 빈자리의 "+"를 누르고 이름을 입력한 뒤 폴더와 옵션을 고릅니다. bridge가 터미널을 열어 `claude`를 실행합니다. tmux를 쓰면 세션 이름은 `sym-<id>`이고, `tmux attach -t sym-<id>`로 그 세션에 들어갈 수 있습니다.

**터미널에서**: 계정을 만들어 token을 받은 다음, 그 token으로 `claude`를 시작합니다:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 응답에 token이 들어 있습니다
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

권한 모드는 `manual`을 추천합니다. `auto` 모드에서는 대부분의 작업을 아무에게도 묻지 않기 때문에 페이지까지 오는 승인이 거의 없습니다. bridge가 기본 주소에 있지 않다면 `SYMPHONY_BRIDGE=http://host:port`도 설정하세요. mod 메시지의 언어는 `SYMPHONY_LANG`으로 고릅니다.

## 계정과 자리

- symphony에서 신원은 이름, token, 고정된 자리로 이루어집니다. 계정 하나는 한 번에 세션 하나만 쓸 수 있습니다. 두 번째 세션은 거부되고, 그 세션의 승인은 터미널에 남습니다.
- Claude 로그인은 여기서 관리하지 않습니다. 모든 세션이 Claude 로그인 하나를 함께 써도 됩니다.
- 데이터는 `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS), `%AppData%\symphony` (Windows)에 저장됩니다. `accounts.json`에는 token이 들어 있어서 소유자만 읽을 수 있습니다(0600). Windows에서는 사용자 폴더의 권한이 이 파일을 보호합니다.

## 보안

- bridge는 이 컴퓨터를 향한 요청만 받고, 다른 웹사이트가 여러분의 브라우저를 통해 보내는 쓰기 요청은 거부합니다. 페이지에서 세션을 시작할 수 있다는 것은 곧 이 컴퓨터에서 명령을 실행할 수 있다는 뜻이기 때문입니다.
- 페이지에 아직 로그인 기능이 없으므로 bridge를 이 컴퓨터 외의 주소에 **바인딩하지 마세요**.

## 플랫폼

| 시스템 | 페이지에서 시작한 세션의 터미널 | 테스트 범위 |
| --- | --- | --- |
| Linux | tmux가 있으면 tmux, 없으면 그래픽 터미널 | 엔드투엔드 테스트 전체 |
| macOS | 기본은 Terminal.app, `-terminal tmux`로 tmux 사용 | 크로스 컴파일과 단위 테스트만 |
| Windows | 새 콘솔 창 | 크로스 컴파일과 단위 테스트만 |

"이 폴더 신뢰"를 대신 눌러 줄 수 있는 것은 tmux뿐입니다. 터미널 창을 쓰는 경우, 새 폴더에서 여는 첫 세션은 그 창에서 한 번 직접 눌러 줘야 합니다.

## 개발

```bash
go test -race ./...                         # bridge와 번역 단위 테스트
claude plugin test mods/symphony-bridge     # mod 단위 테스트
claude plugin validate .                    # 마켓플레이스와 mod 검사
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 엔드투엔드: 실제 세션을 띄우고 token을 씁니다
```

- mod 작업: `go run ./cmd/bridge -mod mods/symphony-bridge`로 실행하면 페이지에서 시작한 세션이 `--plugin-dir`로 여러분의 mod 사본을 받습니다. 직접 시작하는 세션에는 `--plugin-dir mods/symphony-bridge`를 손수 붙이세요.
- 페이지 작업: `go run ./cmd/bridge -web web`으로 실행하면 새로고침만으로 수정 내용이 보입니다.
- 실제 세션 없이 사무실 보기: 두 번째 bridge를 띄워 가짜 활동으로 채웁니다. `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`를 실행한 다음 `go run ./cmd/demo -base http://127.0.0.1:7799`를 실행하세요.
- 번역: 언어마다 파일이 하나씩 있습니다. 페이지용은 `web/locales/<lang>.json`, bridge용은 `internal/i18n/locales/<lang>.json`이고 키는 `en.json`과 같습니다. mod의 몇 안 되는 메시지는 `mods/symphony-bridge/hooks/messages.ts`에 있습니다. 단위 테스트는 모든 언어에 모든 키와 같은 `{플레이스홀더}`가 있는지 확인하고, 엔드투엔드 테스트 `TestI18n`은 모든 언어로 페이지를 열어 봅니다.
- 엔드투엔드 테스트에는 claude, tmux, chromium이 필요하므로 Linux와 tmux를 설치한 macOS에서 돌아갑니다. 기본적으로 이 저장소의 mod를 쓰고, `SYMPHONY_E2E_MOD=installed`로 설정하면 설치된 mod를 씁니다. 스크린샷은 `e2e/artifacts/`에 저장됩니다.
- 릴리스: main에 있는 커밋에 버전 태그를 달아 push하면(`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`) GitHub Actions가 모든 플랫폼용 프로그램을 빌드해 릴리스를 게시합니다(`.github/workflows/release.yml`). 태그는 `plugin.json`에 있는 mod 버전과 같아야 합니다. `.github/release.sh v0.3.0 dist`로 같은 압축 파일을 로컬에서 만들 수 있습니다.

| 폴더 | 설명 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod (TypeScript) |
| `.claude-plugin/marketplace.json` | `/plugin install`이 읽는 플러그인 마켓플레이스 |
| `cmd/bridge/`, `internal/bridge/` | bridge |
| `internal/i18n/` | 언어 감지와 bridge 번역 |
| `web/` | 사무실 페이지와 승인 페이지, 그리고 그 번역 (bridge에 내장됨) |
| `cmd/demo/` | 사무실에 보여 줄 가짜 활동 |
| `e2e/` | 엔드투엔드 테스트 |

## 라이선스

[MIT](LICENSE)
