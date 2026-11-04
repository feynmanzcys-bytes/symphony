# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | **한국어** | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

symphony는 독립 프로젝트입니다. Anthropic과 제휴 관계가 없으며, Anthropic의 보증이나 후원을 받지 않습니다. Claude와 Claude Code는 Anthropic, PBC의 상표입니다.

Claude Code 세션들을 한 팀으로 꾸려, 채팅 앱 속 동료처럼 한 페이지에 모아 보세요. 누가 일하고 있고 누가 여러분을 기다리는지 한눈에 보이고, 동료마다 자기 대화에서 이야기할 수 있으며, 권한 승인과 AskUserQuestion 질문도 거기서 클릭 한 번으로 처리할 수 있습니다.

![동료: 왼쪽에 얼굴과 함께 나열된 세션들, 가운데 대화, 오른쪽 프로필과 컴퓨터](docs/coworkers.png)

이것이 symphony가 여는 페이지인 **동료**입니다. 왼쪽에는 세션들이 나열되고 그 얼굴이 무엇을 하는지 보여 주며, 가운데에는 대화, 오른쪽에는 프로필과 컴퓨터가 있고, 팀은 그룹 채팅이 됩니다. 같은 사무실은 **픽셀 오피스**로도 볼 수 있습니다. 세션들이 회사 건물 한 층의 자기 자리에 앉은 직원이 되는 테마로, 설정 → 테마에서 고르면 bridge가 기억합니다.

![픽셀 오피스: 가운데 업무 공간의 자기 자리에 앉은 직원들, 왼쪽에 작은 회의실, 오른쪽에 회의실과 휴게실](docs/office.png)

## 설치

가장 빠른 방법: Claude Code를 열고 아래 문장을 붙여 넣으세요.

> https://github.com/feynmanzcys-bytes/symphony 설치해 줘

Claude Code가 이 README를 읽고 다음 절의 단계를 해 줍니다. 설치가 끝난 뒤에도 남는 변경을 하기 전에는 여러분에게 묻습니다. 직접 설치하려면 같은 단계를 따라 하세요.

### AI 에이전트용: 설치 런북

사용자가 symphony 설치를 부탁했기 때문에 이 글을 읽고 있을 것입니다. 아래 단계를 셸에서 순서대로 실행하고, 마지막에 6단계의 보고를 하세요. 일상적인 선택(압축 파일, `PATH`에 넣을 폴더)은 직접 하되, 이 설치가 끝난 뒤에도 남는 변경을 하기 전에는 사용자에게 물어보세요. 로그인할 때마다 symphony 시작하기, 셸 rc 파일 수정, 사용자의 `PATH` 변경이 그런 변경입니다. 모든 선택은 보고에 적으세요.

symphony는 두 부분으로 되어 있고, 둘 다 있어야 합니다.

1. **`symphony` 프로그램**(bridge): 바이너리 하나로 된 로컬 웹 서버로, `http://127.0.0.1:7788/`에서 사무실 페이지를 제공하고 세션을 실행합니다. 사용자의 `PATH`에 `symphony`라는 이름으로 있는 것이 좋습니다. 세션이 실행 중인 bridge를 찾지 못하면 mod가 `symphony -background`로 bridge를 시작하기 때문입니다.
2. **`symphony-bridge` 플러그인**(mod): 이 저장소의 마켓플레이스에 있는 Claude Code 플러그인으로, 사용자 범위로 설치합니다. `SYMPHONY_TOKEN`을 가지고 시작한 세션에서만 동작하고, 다른 세션에서는 아무것도 하지 않습니다.

#### 1단계: 컴퓨터 확인

```bash
uname -sm                 # Linux/macOS: 아래에서 고를 압축 파일이 정해집니다 (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # Claude Code 2.1.x 필요 (2.1.317에서 테스트)
git --version             # 협업 작업에만 필요. 없으면 보고에 적되 멈추지는 마세요
symphony -version         # 이미 설치돼 있나? "symphony v1.6.0 (darwin/arm64)" 출력
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7788/api/state   # bridge가 실행 중인가? 401: symphony 1.x, 200: 그보다 오래된 버전, 000: 없음
claude plugin list --json # mod가 이미 설치돼 있나? "id": "symphony-bridge@symphony"가 있는 항목을 찾으세요
brew --version            # Homebrew가 있나요(macOS, Linux)? 있으면 2단계에서 그걸로 설치할 수 있음
```

Windows에서는 PowerShell을 쓰세요. `$env:PROCESSOR_ARCHITECTURE`는 `AMD64` 또는 `ARM64`입니다. `scoop --version`이 응답하면 Scoop이 설치되어 있습니다.

`claude`가 없으면 멈추고, 먼저 Claude Code를 설치하라고 사용자에게 알리세요. 프로그램과 mod가 둘 다 이미 있으면 대신 아래 "업데이트"로 가세요. 둘 중 하나만 있으면 없는 쪽의 단계를 진행하세요. 프로그램은 2단계와 3단계(`symphony -version`이 이미 최신 릴리스를 출력하면 건너뜁니다), mod는 4단계이고, 그다음 5단계로 가세요. 이미 실행 중인 bridge는 시작할 때의 프로그램으로 계속 돌아가므로, 실행 중인 bridge가 있으면 5단계 전에 그것을 멈추게 됩니다.

#### 2단계: 프로그램 받기

**Homebrew나 Scoop으로.** 이 저장소는 Homebrew tap이자 Scoop bucket이기도 합니다. 1단계에서 Homebrew(macOS, Linux)나 Scoop(Windows)을 찾았다면, 이 단계의 나머지와 3단계 대신 그걸로 설치할 수 있습니다. 최신 릴리스의 압축 파일을 내려받아 formula나 manifest에 있는 SHA-256(릴리스가 나올 때 그 릴리스의 `checksums.txt`에서 적어 넣은 값)으로 검사하고, `symphony`를 `PATH`에 넣어 줍니다. 그다음 4단계로 넘어가고, 어느 쪽으로 설치했는지 보고에 적으세요.

```bash
brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
brew install symphony
symphony -version          # 버전과 플랫폼 출력
```

```powershell
scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony
scoop install symphony
symphony -version
```

그렇지 않으면 이 컴퓨터에 맞는 압축 파일과 릴리스의 `checksums.txt`를 새로 만든 전용 폴더에 내려받고, 무엇이든 설치하기 전에 압축 파일을 검사하세요. 압축 파일은 `symphony_<os>_<arch>.tar.gz`(`<os>`는 `linux` 또는 `darwin`(macOS), `<arch>`는 `amd64` 또는 `arm64`)와 `symphony_windows_<arch>.zip`입니다.

Linux와 macOS에서는:

```bash
d=$(mktemp -d)   # 자신만 읽을 수 있는 새 폴더
echo "$d"        # 적어 두세요: 3단계에서 여기서 설치합니다
cd "$d"
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/checksums.txt
if command -v sha256sum >/dev/null; then sha256sum -c --ignore-missing checksums.txt; else shasum -a 256 -c --ignore-missing checksums.txt; fi   # 반드시 "symphony_<os>_<arch>.tar.gz: OK"가 출력돼야 합니다
tar xzf symphony_<os>_<arch>.tar.gz              # 이제 프로그램은 "$d/symphony"에 있습니다
```

검사 결과에 `OK`가 나오지 않으면 멈추세요. 프로그램을 설치하지 말고 사용자에게 알리세요. curl 대신 `gh`를 쓴다면 `$d`에 내려받은 다음(폴더가 아직 없으면 gh가 만듭니다), 거기서 같은 검사와 `tar`를 실행하세요.

```bash
gh release download -R feynmanzcys-bytes/symphony -p 'symphony_<os>_<arch>.tar.gz' -p checksums.txt -D "$d"
```

Windows에서는 PowerShell로:

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$zip = "symphony_windows_$arch.zip"
$d = Join-Path $env:TEMP ("symphony-" + [guid]::NewGuid())   # 새 폴더
New-Item -ItemType Directory -Path $d | Out-Null
$url = 'https://github.com/feynmanzcys-bytes/symphony/releases/latest/download'
Invoke-WebRequest -UseBasicParsing -Uri "$url/$zip" -OutFile (Join-Path $d $zip)
Invoke-WebRequest -UseBasicParsing -Uri "$url/checksums.txt" -OutFile (Join-Path $d 'checksums.txt')
$want = ((Select-String -SimpleMatch -Path (Join-Path $d 'checksums.txt') -Pattern $zip).Line -split '\s+')[0]
$got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $d $zip)).Hash
if ($got -ne $want) { throw "checksum mismatch: $zip" } else { 'checksum OK' }
```

그다음, `gh`가 설치되어 있고 로그인되어 있다면(`gh auth status`가 성공하면) 압축 파일이 어디서 빌드됐는지 확인하세요. 릴리스의 모든 압축 파일에는 이 저장소의 릴리스 workflow가 빌드했다는 GitHub의 증명(attestation)이 붙어 있습니다. 터미널 밖에서는 확인이 통과해도 `gh`가 아무것도 출력하지 않으니, 종료 상태로 판단하세요.

```bash
gh attestation verify "$d/symphony_<os>_<arch>.tar.gz" --repo feynmanzcys-bytes/symphony && echo 'attestation OK'
```

```powershell
gh attestation verify (Join-Path $d $zip) --repo feynmanzcys-bytes/symphony
if ($LASTEXITCODE -eq 0) { 'attestation OK' }
```

v1.2.0 이전 릴리스에는 증명이 없습니다(`no attestations found`). 그런 릴리스는 SHA-256 검사로만 확인합니다. 그 밖의 실패라면 SHA-256 검사가 실패했을 때처럼 멈추세요. `gh`가 없거나 로그인되어 있지 않으면 이 확인은 건너뛰고, 보고에 그렇게 적으세요.

사용자가 원하면 Linux나 macOS에서 **소스로 빌드**할 수도 있습니다(Go 1.27 이상). 바이너리를 빌드하세요. `-autostart`가 `go run` 빌드를 거부하므로 `go run`으로는 안 됩니다.

```bash
d=$(mktemp -d)
git clone https://github.com/feynmanzcys-bytes/symphony.git "$d/src"
cd "$d/src" && go build -o "$d/symphony" ./cmd/bridge   # 이제 프로그램은 압축 파일에서 받았을 때처럼 "$d/symphony"에 있습니다
```

| 시스템 | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple 실리콘) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

압축 파일마다 프로그램(`symphony`, Windows에서는 `symphony.exe`), `LICENSE`, `README.md`가 들어 있습니다. 릴리스의 `checksums.txt`에는 모든 압축 파일의 SHA-256이 있습니다.

#### 3단계: PATH에 넣기

2단계에서 받은 프로그램 `$d/symphony`를 설치하세요(셸이 명령 사이에 변수를 유지하지 않는다면, 2단계에서 출력한 폴더를 그대로 적으세요). 사용자의 `PATH`에 이미 들어 있고 `sudo` 없이 쓸 수 있는 폴더를 쓰세요. `~/.local/bin`, `/usr/local/bin`, `/opt/homebrew/bin` 순서로 찾습니다.

```bash
install -m 755 "$d/symphony" ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: 바이너리에 서명이 없습니다
symphony -version          # "symphony v1.6.0 (linux/amd64)" 출력: 버전과 플랫폼
```

이 폴더들 중 `PATH`에 있는 것이 하나도 없으면, 무엇이든 바꾸기 전에 사용자에게 물어보세요. 좋다고 하면 `~/.local/bin`을 만들어 셸의 rc 파일(`~/.zshrc` 또는 `~/.bashrc`)에 추가하고, 보고에 그렇게 적으세요. 싫다고 하면 사용자가 말하는 곳에 프로그램을 설치하고, `symphony`가 사용자의 Claude Code 세션의 `PATH`에 없는 동안에는 mod가 bridge를 스스로 시작할 수 없다고 알려 주세요(5단계의 로그인할 때 시작은 그래도 됩니다).

Windows에서는 전용 폴더에 압축을 푸세요(`$d`와 `$zip`은 2단계에서 정한 것입니다. 새 셸이라면 다시 설정하세요).

```powershell
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Expand-Archive -LiteralPath (Join-Path $d $zip) -DestinationPath $dest -Force
& (Join-Path $dest 'symphony.exe') -version   # "symphony v1.6.0 (windows/amd64)" 출력
```

그 폴더를 사용자의 `Path`에 추가하는 것은 계속 남는 변경이므로, 먼저 사용자에게 물어보세요. 좋다고 하면 아래 줄을 실행하고 새 셸을 여세요. 싫다고 하면 전체 경로를 쓰세요. `Expand-Archive`로 푼 프로그램에는 다운로드 표시가 없어서 SmartScreen이 막지 않습니다. 파일 탐색기의 "압축 풀기"로 풀고 두 번 클릭해 실행하면 막힐 수 있습니다. "추가 정보"를 누른 뒤 "실행"을 누르세요.

```powershell
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dest", 'User')
```

이렇게 한 뒤에도 지금 쓰는 셸이 `symphony`를 찾지 못하면(바꾸기 전에 `PATH`를 읽어 두었기 때문입니다), 나머지 단계에서는 절대 경로를 쓰세요.

#### 4단계: mod 설치

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

이 명령은 저장소를 플러그인 마켓플레이스 `symphony`로 추가하고, 아무것도 묻지 않고 mod를 사용자 범위로 설치합니다. 저장소의 로컬 클론을 대신 마켓플레이스로 쓸 수도 있습니다: `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. 그러면 세션은 그 폴더에서 mod를 읽고, 거기서 `git pull`하면 업데이트됩니다. 그러니 그런 클론은 임시 폴더가 아니라 계속 둘 곳에 두세요.

확인:

```bash
claude plugin list --json   # 항목 하나에 "id": "symphony-bridge@symphony", "scope": "user", "enabled": true가 있어야 합니다
```

- `"scope": "project"` 또는 `"local"`: mod가 한 폴더에서만 로드됩니다. `claude plugin uninstall symphony-bridge@symphony`를 실행한 뒤 `--scope user`로 다시 설치하세요.
- `"enabled": false`: `claude plugin enable symphony-bridge@symphony`.

#### 5단계: bridge 시작

1단계에서 실행 중인 bridge를 찾았다면, 아래 "업데이트"처럼 먼저 그것을 멈추세요. 그 bridge는 예전 프로그램으로 돌아가고 있어서, 새 프로그램은 거기서 그것을 발견하고 물러날 뿐입니다. bridge를 멈추면 bridge가 시작한 세션도 끝나므로 먼저 사용자에게 알리세요.

symphony를 로그인할 때마다 시작할지 사용자에게 물어보세요.

그렇다고 하면:

```bash
symphony -autostart on -open=false
```

이 명령은 bridge를 지금 시작하고, 앞으로 로그인할 때마다 시작하게 합니다(Linux에서는 systemd 사용자 서비스, macOS에서는 launchd 에이전트, Windows에서는 레지스트리 Run 키). 그리고 날짜와 시각 뒤에 다음을 출력합니다.

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/?key=<key>
```

아니라고 하면:

```bash
symphony -background -open=false
```

이 명령은 bridge를 터미널과 분리해 시작하고(컴퓨터를 다시 시작한 뒤에는 다시 시작되지 않습니다), 표준 오류에 날짜와 시각 뒤에 다음을 출력합니다.

```
the symphony bridge runs in the background at http://127.0.0.1:7788/?key=<key>; its log: <data folder>/bridge.log
```

이 버전의 bridge가 이미 실행 중이면 대신 (같은 방식으로) `a symphony bridge is already running at http://127.0.0.1:7788/?key=<key>`를 출력합니다. 문제없는 상황이니 또 시작하지 마세요. `<key>`는 브라우저를 사무실에 로그인시키는 페이지 키이므로, 다른 곳에 붙여 넣지 마세요. 프로그램이 `127.0.0.1:7788 is taken: another program, or a symphony bridge with another data folder, answers there`라고 답하면, 거기서 무엇이 응답하는지 사용자에게 알리고(예전 버전의 symphony bridge도 여기에 해당합니다. "업데이트"처럼 멈추세요), 묻지 않고 다른 주소를 고르지 마세요.

요청 하나로 전체 연결을 확인합니다. 이 요청은 데이터 폴더에 있는 페이지 키를 제시합니다(표준 입력으로 넘기므로, 다른 사용자가 볼 수 있는 명령줄에 키가 드러나지 않습니다).

```bash
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-${XDG_CONFIG_HOME:-$HOME/.config}/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options      # Linux
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-$HOME/Library/Application Support/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options   # macOS
```

```powershell
$data = if ($env:SYMPHONY_DATA) { $env:SYMPHONY_DATA } else { Join-Path $env:APPDATA 'symphony' }
(Invoke-WebRequest -UseBasicParsing -Headers @{ 'X-Symphony-Key' = (Get-Content (Join-Path $data 'key')) } -Uri http://127.0.0.1:7788/api/launch/options).Content
```

데이터 폴더는 Linux에서는 `~/.config/symphony`(`$XDG_CONFIG_HOME`이 설정돼 있으면 `$XDG_CONFIG_HOME/symphony`), macOS에서는 `~/Library/Application Support/symphony`, Windows에서는 `%AppData%\symphony`이고, `SYMPHONY_DATA`가 설정돼 있으면 그 폴더입니다.

응답에 `"enabled":true`와 `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"1.1.1"}`가 있으면, bridge가 실행 중이고 `claude`를 찾았으며 세션이 로드할 위치에 mod가 설치된 것입니다. `"enabled":false`이면 `mod.code`가 무엇이 문제인지, `mod.fix`가 그것을 고치는 명령을 알려 줍니다. `mod_not_installed`나 `mod_project_scope`(4단계로 돌아가기), `mod_disabled`(활성화하기), `mod_check_failed`(`claude plugin list --json`이 실패한 것이니 Claude Code가 너무 오래된 버전일 가능성이 큽니다)입니다. `"mod":null`은 bridge가 자기 `PATH`에서 `claude`를 아예 찾지 못했다는 뜻입니다(로그에 나옵니다). claude를 거기에 넣거나 `-claude /path/to/claude`로 bridge를 시작하세요. 응답에 `"code":"page_key"`가 있으면 요청이 키를 제시하지 않았거나 틀린 키를 제시했다는 뜻입니다. `cat`이 키 파일을 찾았는지 확인하세요(데이터 폴더는 위 참고). 찾았다면 실행 중인 bridge가 다른 데이터 폴더로 시작된 것입니다.

bridge의 로그는 데이터 폴더의 `bridge.log`입니다. 로그인할 때 시작된 bridge는 Linux에서는 `journalctl --user -u symphony`, macOS에서는 `~/Library/Logs/symphony.log`에 로그를 남깁니다.

#### 6단계: 사용자에게 보고

`symphony`를 실행해 사무실을 여세요. bridge가 실행 중이면 기본 브라우저에서 이미 로그인된 상태로 사무실을 열고 종료합니다. `?key=`가 붙은 주소도 출력하는데, 이 주소로 이 컴퓨터의 어느 브라우저에서든 로그인할 수 있습니다.

사용자에게 알려 주세요.

- 사무실을 열려면 `symphony`를 실행하세요(또는 더블클릭). 이미 로그인된 브라우저에서는 `http://127.0.0.1:7788/`만으로도 열립니다. 목록 아래의 "+ 동료"를 눌러 첫 직원을 뽑으세요. 이름을 정하면 bridge 안에서 Claude Code 세션이 시작되고, 그 직원의 전용 폴더를 새로 만들어 거기서 일합니다("고급 설정"에서 폴더를 골랐다면 그 폴더). 페이지에 있는 그 직원과의 대화가 곧 그 세션의 채팅입니다. 설정 → 동료 → "모든 동료"(픽셀 사무실에서는 직원)에는 모든 직원이 나오고 새 직원 명단을 가져올 수도 있으며, 설정 → 테마에서 픽셀 오피스로 바꿀 수 있습니다.
- 새 직원은 대화 상자에서 다른 것을 고르지 않는 한 사용자의 Claude Code 설정(모델, 사고 강도, 권한 모드)을 따릅니다. `bypassPermissions`는 모든 명령과 편집을 묻지 않고 실행하므로, 세션이 읽는 내용이 사용자 권한으로 명령을 실행하게 만들 수 있습니다. 이 모드를 고르면 대화 상자에 경고가 나옵니다.
- 세션은 여느 Claude Code 세션처럼 사용자의 Claude 요금제나 API 크레딧을 씁니다. 작업 하나는 세션 2~5개를 동시에 돌립니다.
- 사무실에는 페이지에서 시작한 세션(또는 `SYMPHONY_TOKEN`으로 직접 시작한 세션, 아래 "세션 시작" 참고)만 나옵니다. 이 설치를 하고 있는 세션을 포함해 사용자의 다른 Claude Code 세션은 보이지 않습니다.
- 무엇을 어디에 설치했는지: 프로그램 경로, 사용자 범위의 mod, bridge를 로그인 시 시작으로 했는지 백그라운드로 시작했는지, `PATH`나 rc 파일을 바꿨다면 그 내용, 여러분이 한 선택, 그리고 `git`이 없다면 없다는 사실.
- 모두 되돌리는 방법: 아래 "제거".

규칙: bridge는 이 컴퓨터에서만 수신합니다(이 컴퓨터가 아닌 주소는 거부합니다. 사용자가 다른 포트를 고르지 않는 한 기본값 `127.0.0.1:7788`을 그대로 두세요). `sudo`로 실행하지 말고, 같은 주소나 같은 데이터 폴더로 두 번째 bridge를 시작하지 마세요(symphony가 거부하고, 실행 중인 bridge를 알려 줍니다).

#### 업데이트

2단계처럼 새 릴리스를 내려받으세요. 실행 중인 bridge는 다시 시작할 때까지 예전 프로그램으로 돌아가므로(Windows에서는 실행 중인 프로그램을 바꿔 넣을 수도 없습니다), `symphony -stop`으로 bridge를 멈추고 3단계처럼 프로그램을 바꿔 넣은 다음, 전에 돌던 방식 그대로 다시 시작하세요. 로그인할 때 시작되는 경우에는 `symphony -autostart on -open=false`(이 명령으로 지금 바로 시작도 됩니다), 아니면 `symphony -background -open=false`입니다. bridge가 기본값이 아닌 `-addr`나 `-data`로 실행 중이라면 이 명령마다 같은 값을 붙이세요. `-stop`은 그 데이터 폴더의 bridge만 멈추고(`-addr`에서, 또는 그 폴더의 bridge가 실제로 실행 중인 곳에서), 응답하는 bridge가 없으면 `nothing to stop`이라고 알립니다. bridge를 멈추면 bridge가 시작한 세션도 끝나므로 먼저 사용자에게 알리세요. (`-stop`은 v1.1.0에서 생겼습니다. v1.0.0 bridge를 멈추려면 그 프로세스를 끝내세요. Linux와 macOS에서는 `pkill -x symphony`, Windows에서는 `Stop-Process -Name symphony`입니다.)

Homebrew나 Scoop으로 설치했다면 내려받기와 3단계 대신 `brew update && brew upgrade symphony`(또는 `scoop update; scoop update symphony`)를 쓰고, 그 앞의 `-stop`과 뒤의 다시 시작은 똑같이 합니다. symphony가 로그인할 때 시작되더라도 시작 항목에는 Homebrew의 링크(`bin/symphony`)가 적히고, 업그레이드하면 이 링크가 새 버전을 가리킵니다. 그래서 여기서 `-autostart on`은 지금 다시 시작하는 역할만 합니다.

mod는 `claude plugin update symphony-bridge@symphony`로 따라옵니다(클론을 마켓플레이스로 쓴다면 대신 거기서 `git pull`).

프로그램과 mod는 같은 버전 번호를 씁니다. 릴리스마다 바뀐 점은 [CHANGELOG.md](CHANGELOG.md)에 있습니다.

#### 제거

```bash
symphony -stop                                    # bridge와 그 세션을 멈춥니다. 어떻게 시작했든 상관없습니다 (-addr나 -data가 기본값이 아니면 그 값도 붙이세요)
symphony -autostart off                           # 로그인할 때 더는 시작하지 않습니다 (원래 그렇게 돼 있지 않았으면 그렇다고 알려 줍니다)
claude plugin uninstall symphony-bridge@symphony  # mod
claude plugin marketplace remove symphony         # 마켓플레이스 항목
rm ~/.local/bin/symphony                          # 프로그램 (3단계에서 넣은 위치)
```

3단계에서 셸 rc 파일에 `~/.local/bin`을 추가했다면 그 줄도 지우세요. Windows에서는 PowerShell로:

```powershell
symphony -stop                                               # bridge와 그 세션을 멈춥니다 (-addr나 -data가 기본값이 아니면 그 값도 붙이세요)
symphony -autostart off                                      # 로그인할 때 더는 시작하지 않습니다
claude plugin uninstall symphony-bridge@symphony
claude plugin marketplace remove symphony
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Remove-Item -Recurse -Force $dest                            # 프로그램
$p = ([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -and $_ -ne $dest }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $p, 'User')    # 3단계에서 추가한 Path 항목
```

Homebrew로 설치했다면 `rm` 줄 대신 `brew uninstall symphony`와 `brew untap feynmanzcys-bytes/symphony`를, Scoop이라면 PowerShell의 마지막 네 줄 대신 `scoop uninstall symphony`와 `scoop bucket rm symphony`를 실행하세요.

데이터 폴더(token이 든 계정, 페이지 키, 메시지 기록, 사용량 장부, 작업, 그리고 `lock`, `bridge.addr`, 따로 치워 둔 `*.corrupt-<time>` 같은 파일)는 남습니다. 전부 지우려면 이 폴더도 지우세요. 직원 전용 폴더(홈 폴더의 `symphony-bots` 또는 설정에서 지정한 폴더)도 그 안의 작업과 함께 남습니다. 이 폴더를 지우면 페이지 키도 지워지므로, 다음에 시작할 때 새 키가 만들어지고 모든 브라우저가 다시 로그인해야 합니다. Linux에서 snap이나 flatpak 브라우저를 쓴다면 `open.html`은 그 브라우저 전용 폴더(`~/snap/<snap>/common` 또는 `~/.var/app/<app id>`)에 있으니 그것도 지우세요.

## 주요 기능

- **세션 하나가 채팅 하나**: 목록에서 동료를 누르면 페이지 가운데에 그 동료와의 대화가 열립니다. 위에는 직원 정보, 아래에는 세션 전체가 채팅으로: 입력한 프롬프트, 모델의 생각(접힌 상태, 진행 중에는 주황색 “생각 중…”), 흘러나오는 답변, 도구 호출, 다른 세션에서 온 메시지(보라색). 아래 입력란에 쓰면 그 세션의 프롬프트가 되고, “중지” 버튼으로 끊을 수 있습니다. 모델이 쓴 내용은 Markdown으로 보입니다(제목, 목록, 표, 코드, 링크). 두 답변 사이의 생각과 도구 호출은 한 줄로 접혀, 일하는 동안은 “생각 중… 2분 38초”, 끝나면 “2분 38초 동안 생각함”이 되고, 누르면 단계가 펼쳐집니다.
- **브라우저에서 승인**: 권한 승인(명령 실행, 파일 수정)과 AskUserQuestion 질문은 그 동료와의 대화 속, 요청이 나온 자리에 카드로 놓이며, 그동안에도 입력할 수 있습니다(픽셀 오피스에서는 직원 창의 입력란 위로 올라옵니다). 여러분을 기다리는 동료는 목록 맨 위에서 빨갛게 바뀌고, 탭 제목에는 카드 수가 나옵니다. 권한 카드의 "이 세션 동안 허용"은 세션이 끝날 때까지 같은 종류의 호출을 묻지 않고 통과시킵니다. 같은 종류란 모든 편집, 앞부분이 같은 명령(`npm test`), 또는 같은 도구입니다. MCP 서버의 양식도 카드로 나옵니다. 답은 세션의 터미널에서 하며, 카드가 그 터미널을 창 안에 열어 줍니다. bridge가 꺼져 있거나, 계정을 알아보지 못하거나, 믿을 수 없는 상대이면 세션은 평소처럼 터미널에서 처리하므로, 멈춰 버리는 일은 없습니다.
- **또 하나의 테마, 픽셀 오피스**: 세션 하나하나가 정해진 자리에 앉은 직원입니다. 초록은 쉬는 중, 노랑은 작업 중, 빨강은 여러분의 승인이나 답변을 기다리는 중, 회색은 퇴근입니다. 쉬는 직원은 처음엔 자리에서 휴대폰을 보거나 게임을 하다가 곧 사무실을 돌아다닙니다. 휴게실 소파에서 TV 보기, 테이블 축구, 술래잡기, 원탁에서 수다, 커피, 창밖 구경을 하고, 한참 지나면 낮잠을 자러 가는데 주로 회의실에 숨어서 잡니다. 캐릭터의 덩치는 모델을, 근육은 사고 강도를 따라갑니다. 화면은 큰 회사 건물의 한 층으로, 창을 가득 채웁니다. 가운데에 업무 공간, 왼쪽에 명패가 달린 3~5인용 작은 회의실, 오른쪽에 회의실과 휴게실이 있고, 직원들은 엘리베이터로 출퇴근합니다. 설정에서 픽셀 크기도 4단계로 고를 수 있습니다. 사무실은 그대로이고, 단계가 올라갈수록 더 작은 픽셀과 더 많은 디테일로 그려집니다.
- **누구든 찾기**: 목록 위의 검색창에서 이름, 세션 이름, 폴더로 동료를 찾고, 작업으로 그룹 채팅을 찾을 수 있습니다(`/` 키로 바로 이동). 픽셀 오피스에서는 쉬는 직원을 클릭하면 자기 자리로 돌아가 다음 일이 올 때까지 거기 있고, 검색창은 위쪽 바에 있으며, 그 옆의 상태 숫자(쉬는 중, 작업 중, 대기 중, 퇴근)를 클릭하면 그 상태의 직원이 모두 강조됩니다. 바 아래의 손잡이로 바를 위로 접었다가 다시 펼 수 있습니다.
- **세션 간 메시지**: 세션끼리 메시지를 보내면(SendMessage) 그 메시지가 양쪽 대화에 보라색으로 나옵니다. 픽셀 오피스에서는 편지 봉투도 사무실을 가로질러 날아가고, 메시지 기록에 남습니다.
- **페이지에서 세션 시작, 창은 뜨지 않음**: "+ 동료"(픽셀 오피스에서는 빈자리의 "+")를 눌러 새 직원을 뽑고, 이름을 정해 시작하세요. 어디서 어떻게 일할지는 "고급 설정" 안에 접혀 있고, 무엇을 쓸지 한 줄로 보여 줍니다. 설정 → 직원에서 정한, 새 직원이 시작할 때의 설정 그대로입니다. 폴더는 기본 폴더(홈 폴더의 `symphony-bots`)에 새로 만든 전용 폴더로, git 저장소로 초기화되고 다음에도 그곳으로 돌아옵니다. 모델, 사고 강도, 권한 모드는 다른 것을 고르지 않으면 여러분의 Claude Code 설정("기본값")을 따릅니다. 사무실에 또 누가 있는지 알려 줄지도 여기서 정하는데, 알려 주면 대화마다 한 번, 온라인인 다른 세션들의 이름이 세션의 시스템 프롬프트에 들어가므로 SendMessage로 그들에게 연락할 수 있습니다. 펼치면 이번 시작에 한해 무엇이든 바꾸거나 폴더를 고를 수 있습니다. `bypassPermissions` 권한 모드를 고르면 접혀 있든 펼쳐져 있든 그것이 무엇을 허용하는지 빨간 경고가 나옵니다. 퇴근한 직원과의 대화에서 "출근"을 누르면 마지막으로 일할 때 고른 그대로 돌아옵니다(픽셀 오피스에서는 원래 자리로). 세션은 bridge 안의 전용 터미널에서 돌아가므로 창이 뜨지도, 포커스를 빼앗기지도 않습니다. 대화 옆의 컴퓨터가 그 터미널을 실시간으로 보여 주고, "인수"를 누르면 그 터미널이 별도의 창에서 열리며(픽셀 오피스에서는 직원 창의 "터미널 열기"), 그 창을 닫아도 아무것도 바뀌지 않습니다.
- **모든 직원을 한 목록에**: 설정 → 동료 → "모든 동료"(픽셀 사무실에서는 직원)에는 모든 직원이 상태, 폴더, 시작할 때 쓰는 설정(모델, 사고 강도, 권한 모드, 사무실 정보)과 함께 나옵니다(페이지에서 한 번도 시작하지 않은 직원은 시작할 때까지 설정을 따릅니다). 한 명은 그 줄에서 편집하고, 여러 명은 선택해서 한꺼번에 바꾸며, 선택한 직원 중 퇴근한 직원은 삭제할 수 있습니다. "가져오기…"는 붙여 넣거나 파일로 고른 명단(한 줄에 이름 하나, 또는 "내보내기"가 만든 JSON)을 받아 무엇을 할지 항목마다 먼저 보여 주고, 새 직원은 만들고 나머지는 바꾸는 일을 한 번에 처리합니다. 잘못된 항목이 하나라도 있으면 아무것도 하지 않습니다. 바꾼 내용은 그 직원이 다음에 시작할 때부터 적용되며, 이름은 그대로입니다.
- **팀은 그룹 채팅으로**: "+ 그룹"(픽셀 오피스에서는 비어 있는 작은 회의실)을 누르고 작업을 적은 뒤 멤버를 2~5명 고릅니다. 멤버는 직원(각자 이 작업만 하는 두 번째 세션을 받습니다)이나 이번 한 번만 함께하는 어드바이저입니다. 기본으로는 첫 번째 멤버가 리더가 되어 SendMessage로 일을 나눠 줍니다. 멤버마다 자기 git worktree의 자기 브랜치에서 일하므로 서로의 파일을 건드릴 일이 없습니다. 픽셀 오피스에서는 작업이 끝날 때까지 멤버들이 회의실에 앉아 있습니다.
- **설정과 문제 보고**: 목록 위의 톱니바퀴(픽셀 오피스에서는 위쪽 바의 '설정' 버튼)에는 가끔 바꾸는 것들이 있습니다: 언어, 테마, 밝게 또는 어둡게, 픽셀 크기, 움직임 멈추기, 단계를 펼쳐서 보기, 그리고 새 직원이 시작할 때 쓰는 설정(폴더, 모델, 사고 강도, 권한 모드, 사무실 정보. 새 팀의 어드바이저와 권한 모드도 여기 설정대로 시작합니다). 문제가 생기면(시작이나 작업 실패, 세션의 API 오류, 페이지 안의 오류) 오류 로그에 저절로 기록됩니다. 보고하고 싶을 때는 설정에서 '문제 보고하기'를 누르고, 받은 파일을 이메일로 보내거나 GitHub 이슈에 첨부하세요. 파일에는 오류와 버전, bridge의 최근 로그가 들어가고 키, 토큰, 대화 내용은 들어가지 않습니다.
- **10개 언어**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil), Русский를 지원합니다. 페이지는 브라우저 언어를 따르고 설정에서 언어를 바꿀 수 있습니다. bridge 메시지는 `-lang`을, 없으면 시스템 언어(`LANG` 등)를, 그것도 없으면 영어를 따릅니다. mod 메시지는 `SYMPHONY_LANG`(페이지에서 시작한 세션이면 그 페이지의 언어)을, 없으면 시스템 언어를, 그것도 없으면 영어를 따릅니다.
- **로컬 전용**: bridge는 이 컴퓨터에서만 수신하는 Go 프로그램 하나(표준 라이브러리만 사용하고, 엔드투엔드 테스트에만 chromedp를 씁니다)이고 페이지는 순수 JavaScript이며, 외부 서비스에 전혀 의존하지 않습니다.

![동료 테마의 대화: 채팅으로 보이는 세션과, 요청이 나온 자리에 놓인 질문 카드와 권한 카드](docs/approve.png)

![설정 → 모든 동료: 모든 직원과 각자의 폴더, 시작할 때 쓰는 설정, 그중 두 명은 한꺼번에 바꾸려고 선택된 상태](docs/staff.png)

## 요구 사항

- Claude Code 2.1.317 (이 버전에서 테스트했습니다). mod는 Claude Code의 function hooks를 쓰는데, 아직 얼리 액세스 단계의 API이므로 Claude Code를 업그레이드한 뒤에는 엔드투엔드 테스트를 돌려 보세요.
- bridge는 Linux, macOS, Windows용으로 미리 빌드된 프로그램이 있습니다. Go 1.27 이상은 소스에서 빌드할 때만 필요합니다.
- `git`: 협업 작업에만 필요합니다. 작업의 멤버는 저마다 git worktree에서 일합니다.
- 그 외에는 아무것도 필요 없습니다. 페이지에서 시작한 세션은 모든 플랫폼에서 bridge 안에서 돌아갑니다.

## bridge 실행

`symphony`는 bridge를 시작하고 브라우저에서 이미 로그인된 상태로 사무실을 엽니다(`-open=false`로 끌 수 있습니다). 브라우저는 데이터 폴더에 있는, 여러분만 읽을 수 있는 파일 `open.html`을 통해 bridge의 페이지 키를 쿠키로 받으므로, 키가 명령줄에 드러나지 않습니다. Linux에서 브라우저가 snap이나 flatpak이면 그런 브라우저는 데이터 폴더를 읽지 못하므로, 그 파일은 대신 브라우저 전용 폴더에 들어갑니다. 로그인되지 않은 브라우저에서 <http://127.0.0.1:7788/>을 열면 들어가는 방법을 알려 주는 페이지가 나옵니다. `symphony`를 다시 실행하거나, bridge가 로그에 출력한 `?key=`가 붙은 주소를 여세요. 승인만 처리하는 더 간단한 페이지는 <http://127.0.0.1:7788/approve>에 있습니다. 실행 중일 때 `symphony`를 다시 시작하면 사무실만 엽니다. Windows에서 `symphony.exe`를 더블클릭하면 별도의 창에서 실행되며, 그 창을 닫으면 bridge도 멈춥니다.

| 플래그 | 기본값 | 설명 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | 수신 대기할 주소. 호스트는 `127.0.0.1`, `localhost`, `::1`(`[::1]:7788`처럼) 중 하나여야 하며, bridge는 다른 호스트를 거부합니다 |
| `-data` | `$SYMPHONY_DATA`, 없으면 설정 폴더 안의 `symphony` | 데이터 폴더: token이 든 계정(`accounts.json`), 페이지 키(`key`)와 `open.html`, 메시지 기록(`messages.json`), 사용량 장부(`ledger.json`), 작업(`tasks.json`), bridge 설정(`settings.json`), 오류 로그(`errors.json`), `-background`의 로그(`bridge.log`), `lock`과 `bridge.addr`(어느 bridge가 어디서 쓰고 있는지), 그리고 손상되어 따로 치워 둔 메시지 기록이나 사용량 장부, 설정(`*.corrupt-<time>`) |
| `-claude` | `claude` | claude 실행 파일 |
| `-lang` | 시스템 언어, 없으면 `en` | bridge 메시지 언어(`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES`, `LANG`으로 기본값이 정해집니다) |
| `-mod` | 비어 있음 | 개발용: 이 폴더의 mod를 `--plugin-dir`로 세션에 넘겨, 설치된 사본 대신 쓰게 합니다 |
| `-web` | 비어 있음 | 개발용: 이 폴더의 페이지를 제공해서, 수정한 내용이 새로고침하면 바로 보입니다 |
| `-open` | `true` | bridge가 시작되면 브라우저에서 사무실을 엽니다. `-addr`에서 이 데이터 폴더의 bridge가 이미 실행 중이면 그 사무실만 엽니다 |
| `-autostart` | — | `on`: 로그인할 때마다 bridge를 자동으로 시작하고(함께 준 플래그 포함) 지금 바로 시작합니다. `off`: 되돌립니다 |
| `-background` | — | bridge를 백그라운드에서 시작합니다. 터미널과 분리되고 로그는 데이터 폴더의 `bridge.log`에 남기며, 응답하면 돌아옵니다 |
| `-stop` | — | 이 데이터 폴더의 bridge를 멈춥니다(`-addr`에서, 아니면 실제로 실행 중인 곳에서). 어떻게 시작했든 상관없고(그 세션도 끝납니다), bridge가 사라지면 돌아옵니다. 키는 그 키를 가지고 있다고 증명하는 bridge에만 보여 줍니다 |
| `-version` | — | 버전을 출력하고 종료합니다 |

`-addr`에서 다른 것(다른 프로그램, 다른 데이터 폴더를 쓰는 bridge, 또는 예전 버전의 symphony bridge)이 응답하면, symphony는 그렇다고 알리고 멈춥니다. 페이지 키는 그 키를 가지고 있다고 증명하는 bridge에만 보여 줍니다. 데이터 폴더 하나는 한 번에 bridge 하나만 쓸 수 있습니다. 지정한 데이터 폴더의 bridge가 이미 다른 주소에서 실행 중이면, symphony는 그 주소를 알리고 그 사무실을 연 뒤 멈춥니다(두 번째 bridge에는 따로 `-data`를 주세요).

**로그인할 때 시작**: `symphony -autostart on`을 실행하면 이후 로그인할 때마다 bridge가 자동으로 시작되고(함께 준 플래그, 예를 들어 `-addr`도 그대로 씁니다) 지금 바로 시작됩니다. `symphony -autostart off`로 되돌릴 수 있습니다. Linux에서는 systemd 사용자 서비스(로그는 `journalctl --user -u symphony`), macOS에서는 launchd 에이전트(로그는 `~/Library/Logs/symphony.log`), Windows에서는 레지스트리 Run 키의 값입니다(bridge는 콘솔 창에서 실행되며, 창을 닫으면 다음 로그인 때까지 멈춥니다). `symphony -stop`은 어떻게 시작했든 bridge를 멈추며, bridge는 다시 시작될 때까지(로그인할 때 시작되는 bridge라면 다음 로그인 때까지) 멈춰 있습니다. `go run`이 아니라 symphony 프로그램 자체가 필요합니다. 로그인할 때 시작되는 bridge는 셸에서 `PATH` 말고는 아무것도 받지 못합니다. 그래서 `-autostart on`은 세션에 필요할 수 있는 변수들도, 그 명령을 실행하는 셸에 있는 그대로 데이터 폴더의 `autostart-env.json`(여러분만 읽을 수 있는 파일)에 저장해 둡니다. `CLAUDE_CONFIG_DIR`, 프록시(`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, `ALL_PROXY`, 대문자든 소문자든), `NODE_EXTRA_CA_CERTS`, `SSL_CERT_FILE`, `SSL_CERT_DIR`이고, 그 밖에는 아무것도 저장하지 않습니다(API 키도 마찬가지입니다). 이 변수들을 바꾼 뒤에는 `-autostart on`을 다시 실행하세요. 시작하자마자 실패하는 bridge(데이터 폴더를 다른 bridge가 쓰고 있거나, `accounts.json`이 손상됐거나, 그 주소를 다른 것이 차지한 경우)는 몇 번 다시 시작해 보고, 그 뒤에는 멈춘 채로 둡니다. Linux에서는 1분 안에 5번(이유는 `systemctl --user status symphony`가 알려 줍니다), macOS에서는 1분에 한 번(이유는 로그에 나옵니다)입니다. 원인을 고친 뒤 `symphony -autostart on`을 다시 실행하거나, 그냥 다시 로그인하세요.

**소스에서** 실행하려면 Go 1.27 이상이 필요합니다: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## 세션 시작

**페이지에서**: "+ 동료"(픽셀 오피스에서는 빈자리의 "+")를 누르고 이름을 입력한 뒤, 원하면 폴더와 옵션을 고릅니다. bridge는 자기 안의 전용 터미널에서 `claude`를 실행하며 창은 열리지 않습니다. 직원과의 대화에 세션이 채팅으로 보이고, 컴퓨터 아래의 "인수"(픽셀 오피스의 직원 창에서는 "터미널 열기")는 그 세션에 연결된 터미널 창을 엽니다(아무 터미널에서나 `symphony attach <id>`로도 됩니다. bridge가 기본값을 쓰지 않으면 `-addr`와 `-data`를 붙이며, 터미널 창을 열 수 없을 때는 정확한 명령이 페이지에 나옵니다. `Ctrl-]`나 창 닫기로 분리되며 세션은 계속됩니다). 세션은 bridge의 자식 프로세스이므로 bridge를 멈추면 세션도 끝납니다. 세션이 끝나면(페이지에서 멈추든, bridge를 멈춰서든, 스스로 종료하든) 세션이 백그라운드에 남겨 둔 것(예를 들어 도구가 `&`로 백그라운드에 돌린 명령)도 함께 끝납니다. Windows에서는 세션의 job object가 그것들을 모두 끝냅니다. Linux와 macOS에서는 bridge가 프로세스의 환경 변수에 든 launch id로 그런 프로세스를 찾습니다. 다만 macOS에서는 Apple 자체 프로그램(`/bin/sleep`, `bash`, `zsh`, `perl`)의 환경 변수를 시스템이 `ps -E`에서 숨기므로 읽을 수 없습니다. 그래서 남겨진 `node`나 Homebrew 서버는 끝나지만, 평범한 `sleep`이나 셸 스크립트는 끝나지 않습니다.

**터미널에서**: 페이지 키를 제시해 계정을 만들고 token을 받은 다음, 그 token으로 `claude`를 시작합니다.

```bash
printf 'X-Symphony-Key: %s\n' "$(cat ~/.config/symphony/key)" | curl -H @- -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # 응답에 token이 들어 있습니다
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

키 파일은 Linux에서는 `~/.config/symphony/key`, macOS에서는 `~/Library/Application Support/symphony/key`, Windows에서는 `%AppData%\symphony\key`이고, `-data` / `SYMPHONY_DATA`로 폴더를 지정했다면 그 폴더 안의 `key`입니다.

권한 모드는 `manual`을 추천합니다. `auto` 모드에서는 대부분의 작업을 아무에게도 묻지 않기 때문에 페이지까지 오는 승인이 거의 없습니다. bridge가 기본 주소에 있지 않다면 `SYMPHONY_BRIDGE`도 설정하세요. 이 컴퓨터의 `http://127.0.0.1:<port>`, `http://localhost:<port>`, `http://[::1]:<port>` 중 하나이거나 `https://` 주소여야 하며, 그 밖의 주소이면 mod는 bridge에 전혀 연결하지 않습니다. mod 메시지의 언어는 `SYMPHONY_LANG`으로 고릅니다(없으면 시스템 언어, 그것도 없으면 영어).

세션이 token을 어디로든 보내기 전에, mod는 그 주소의 bridge가 token을 알고 있는지를 token을 보내지 않고 확인합니다(`POST /api/hello`). bridge가 세션을 받을 수 없으면 세션은 아래 메시지 중 하나를 보여 주고, 승인은 터미널에 남으며, 멈춰 버리는 일은 없습니다.

| 메시지 | 뜻 |
| --- | --- |
| `bridge(...)에 연결할 수 없습니다` / `can't reach the bridge` | bridge 주소에서 아무것도 응답하지 않고, bridge를 시작해도 소용이 없었습니다(또는 그 주소가 이 컴퓨터가 아닙니다). |
| `bridge(...)를 시작하지 못했습니다: <reason>` / `couldn't start the bridge ...: <reason>` | 아무것도 응답하지 않았고, `symphony -background`도 표시된 이유로 실패했습니다(예를 들어 이 데이터 폴더의 bridge가 다른 주소에서 실행 중이라면 `SYMPHONY_BRIDGE`를 그 주소로 설정하세요). |
| `bridge가 이 SYMPHONY_TOKEN을 알지 못합니다` / `the bridge doesn't know this SYMPHONY_TOKEN` | bridge에 이 token을 가진 계정이 없습니다. 계정이 삭제됐거나, bridge가 다른 데이터 폴더를 쓰고 있습니다. |
| `이 세션이 신뢰할 수 있는 symphony bridge가 아니거나` / `isn't a symphony bridge this session can trust` | 그 주소에서 무언가 응답하지만 token을 안다는 것을 증명하지 못합니다. 다른 프로그램이거나, mod보다 오래된 symphony입니다(업데이트하세요). token은 보내지 않습니다. |
| `SYMPHONY_BRIDGE(...)는 이 컴퓨터에 있거나 https를 사용해야 합니다` / `SYMPHONY_BRIDGE ... must be on this machine or use https` | mod가 받아들이지 않는 주소입니다. bridge에는 전혀 연결하지 않습니다. |
| `이 계정에는 이미 온라인 세션이 있습니다` / `this account already has a session online` | 계정 하나는 한 번에 세션 하나만 쓸 수 있습니다. 이 세션용으로 계정을 하나 더 만드세요. |
| `사무실 웹 페이지(...)에서 응답을 기다리는 중입니다` / `waiting for your answer in the office page` | 권한 승인이나 질문이 터미널이 아니라 사무실 페이지에서 기다리고 있습니다. 거기서 답하거나, Esc를 눌러 취소하세요. 세션마다 한 번만 표시됩니다. |

이 컴퓨터의 bridge 주소에서 아무 응답이 없으면, 세션이 `symphony -background -open=false -addr <that address>`로 bridge를 직접 시작하고(`symphony`가 PATH에 있을 때) 그렇다고 알립니다(`백그라운드에서 bridge(...)를 시작했습니다` / `started the bridge ... in the background`). bridge는 세션이 끝나도 계속 실행됩니다. 계정이 기본 데이터 폴더에 없다면 `SYMPHONY_DATA`를 설정하세요. 이 데이터 폴더의 bridge가 이미 다른 주소에서 실행 중이면 그렇다고 알리고, 세션은 터미널에서 처리합니다. `SYMPHONY_BRIDGE`를 그 주소로 설정하세요.

## 협업 작업

작업은 세션 2~5개가 한 팀이 되어 한 가지 일을 함께 하는 것으로, 동료 테마에서는 그룹 채팅, 픽셀 오피스에서는 작은 회의실입니다. 작업에는 `git`이 필요합니다.

**작업 시작**: "+ 그룹"(픽셀 오피스에서는 비어 있는 작은 회의실이나 위쪽 바의 "새 작업")을 누르고 할 일을 적습니다. 원하는 순서대로 멤버를 체크하고(2~5명), 멤버마다 모델과 사고 강도를, 팀 전체의 권한 모드를 고른 뒤(기본은 Claude Code 자체 설정입니다. 전에 일한 적 있는 직원은 마지막으로 쓴 모델과 사고 강도로 시작하고, `bypassPermissions`를 고르면 경고가 나옵니다) "작업 시작"을 누릅니다. 멤버는 둘 중 하나입니다.

- **직원**: 이 작업만 하는 두 번째 세션을 받아 그 직원의 폴더에서 일합니다. 자리에 있는 원래 세션은 그대로입니다. 직원은 한 번에 작업 하나에만 들어갈 수 있고, 한 번도 일한 적 없는 직원은 아직 폴더가 없으니 먼저 "출근"을 한 번 누르거나, 설정 → 동료 → "모든 동료"(픽셀 사무실에서는 직원)에서 폴더를 정해 주세요.
- **어드바이저**: 이 작업을 위해 늘 쓰던 항목(이름, 폴더, 모델, 사고 강도)으로 만드는 일회성 멤버입니다. 자리가 없고(픽셀 오피스에서는 엘리베이터로 올라옵니다) 작업이 끝나면 사라집니다.

**리더**: 기본으로는 첫 번째 멤버가 리더입니다. 작업을 나누고, SendMessage로 각자 맡을 부분을 나눠 주고, 결과를 검토하고, 다른 멤버의 브랜치를 자기 브랜치에 병합한 뒤 무엇을 했는지 알려 줍니다. 리더 체크를 풀면 모두가 작업 전체를 받고, 멤버끼리 나눠 맡습니다. 어느 쪽이든 각 멤버의 첫 프롬프트는 bridge가 보냅니다. 거기에는 팀에 누가 있는지(멤버끼리는 SendMessage로 이름을 대고 연락합니다), 모두의 브랜치, 그리고 규칙이 들어 있습니다.

**git**: 멤버마다 자기 폴더의 전용 git worktree에서 자기 브랜치 `symphony/<task id>/<member>`(예: `symphony/T1/bob`. 같은 이름의 멤버가 또 있으면 `-2`가 붙습니다) 위에서 일하므로, 서로의 파일을 건드릴 일이 없고 여러분의 체크아웃도 그대로입니다. worktree는 폴더 안의 `.symphony/worktrees/`에 있고, `.git/info/exclude`로 git에서 숨깁니다(여러분의 `.gitignore`는 건드리지 않습니다). 작업이 끝나면 멤버가 커밋하지 않은 변경은 그 멤버의 브랜치에 커밋되고, worktree에서 git이 무시하는 파일(빌드 결과물, `.env`, `node_modules` 등)은 폴더 안의 `.symphony/kept/<member>-<task id>/`로 옮겨지며, worktree는 지워집니다. symphony는 `.symphony/kept/`를 절대 지우지 않습니다. 안에 든 것이 더는 필요 없으면 직접 지우세요.

- 여러분의 저장소에서는 아무것도 병합하지 않습니다. 브랜치가 남아 있으니 직접 병합하세요. 리더가 있다면 리더의 브랜치에 모두의 결과가 모여 있습니다.
- git 저장소가 아닌 폴더는 작업을 위해 저장소가 되고, 그 뒤에도 저장소로 남습니다. `git init`을 한 다음, 비밀 정보가 들어 있는 경우가 많거나 덩치가 큰 파일(`.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa*`, `id_ecdsa*`, `id_ed25519*`, `.npmrc`, `.netrc`, `.pypirc`, `node_modules/`, `.DS_Store`)을 빼고 안의 모든 것을 커밋하며, 이 목록은 `.git/info/exclude`에 적습니다. 그런 파일은 폴더에 그대로 있지만 멤버의 worktree에는 없으므로, 필요한 멤버는 직접 설치하거나 만듭니다. 작업이 끝나면 멤버의 브랜치가 폴더의 브랜치에 병합되어 폴더에 결과가 담깁니다. `.git`과 브랜치는 그대로 남습니다. 작업이 시작되지 못하면 폴더는 다시 git 저장소가 아닌 원래 폴더로 돌아갑니다.
- 병합에 실패하면 그 병합은 되돌려져 기준 브랜치는 병합 전 그대로이고, 브랜치는 작업 내용과 함께 남으며, 어느 브랜치가 충돌했는지 작업 창에 보여 줍니다. 작업이 끝나는 동안 그 밖에 실패한 것은 그 상태 그대로 남고, 무엇이 실패했는지 작업 창에 보여 줍니다.

**동료 테마에서는**: 작업은 목록에서 동료들 아래에 있는 그룹 채팅입니다. 누르면 가운데에 작업 창이 열립니다. 작업 내용, 멤버와 각자의 폴더와 브랜치, 팀 채팅, 그리고 "작업 끝내기"가 있고, 오른쪽에는 멤버들의 얼굴과 상태가 나옵니다. 멤버를 누르면 그 멤버와의 대화가 열리고, 거기서 그룹으로 돌아갈 수 있습니다. 멤버가 여러분을 기다리면 그룹이 빨갛게 바뀝니다.

**픽셀 오피스에서는**: 작업하는 동안 멤버들은 회의실 탁자에 앉아 있고, 직원의 자리에는 빈 의자만 남습니다. 회의실 명패에는 작업 ID가 나옵니다. 명패의 불빛은 비어 있을 때는 초록, 작업 중에는 노랑, 멤버가 여러분의 승인이나 답변을 기다릴 때는 빨강입니다. 회의실을 누르면 작업 창이 열립니다. 작업 내용, 멤버와 각자의 폴더와 브랜치, 팀 채팅, 그리고 "작업 끝내기"가 있습니다. 회의실에 있는 멤버를 눌러도 작업 창이 열립니다. 작업 창에서 멤버를 누르면 그 멤버의 채팅 창이 작업 창 옆에 붙어서 열리는데, 직원 창과 똑같고 한 번에 한 명만 열립니다(다른 멤버를 누르면 바뀝니다). 상단 바의 빨간 숫자나 검색창으로 작업 멤버를 열 때도 마찬가지입니다.

**팀 채팅**: 작업 창에는 멤버끼리 주고받은 메시지가 나옵니다. 메시지마다 보낸 멤버의 얼굴(픽셀 오피스에서는 픽셀 캐릭터)과 이름이 붙고, 본문은 `@`와 받는 사람으로 시작합니다. 아래 입력란에서 여러분도 팀에 직접 쓸 수 있습니다. `@bob-T1 …`은 그 멤버에게만 가고(`@names`를 여러 개 쓰면 그 모두에게 갑니다. `@`를 치면 멤버 목록이 열리고, 화살표 키와 Enter 또는 클릭으로 고릅니다), `@` 없는 메시지는 작업의 모든 멤버에게 갑니다. Enter로 보내고 Shift+Enter로 줄을 바꿉니다.

작업은 작업 창에서 끝내거나 마지막 세션이 끝나면 끝납니다. bridge는 멤버의 세션을 멈추고, 위에서 말한 대로 git 쪽을 정리하고, 어드바이저를 지웁니다. bridge가 멈출 때 아직 진행 중이던 작업은 다음에 bridge가 시작될 때 같은 방식으로 끝납니다.

## 계정과 자리

- symphony에서 신원은 이름, token, 고정된 자리로 이루어집니다. 계정 하나는 한 번에 세션 하나만 쓸 수 있고, 참여 중인 작업에서 하나를 더 쓸 수 있습니다. 그 밖의 세션은 거부되고, 그 세션의 승인은 터미널에 남습니다.
- 설정 → 동료 → "모든 동료"(픽셀 사무실에서는 직원)에는 모든 직원(어드바이저는 자기 작업에 속하므로 나오지 않습니다)이 페이지에서 시작할 때 쓰는 설정과 함께 나오고, 여기서 한 명 또는 여러 명의 설정을 바꾸고, 명단에서 가져오고, 내보낼 수 있습니다. token은 절대 보여 주지 않습니다(token은 `POST /api/accounts`의 응답에 한 번만 나옵니다).
- Claude 로그인은 여기서 관리하지 않습니다. 모든 세션이 Claude 로그인 하나를 함께 써도 됩니다.
- 데이터는 `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS), `%AppData%\symphony` (Windows), 또는 `-data` / `SYMPHONY_DATA`로 지정한 곳에 저장됩니다. `accounts.json`에는 token이, `key`에는 페이지 키가 들어 있고, 둘 다 소유자만 읽을 수 있습니다(0600). Windows에서는 사용자 폴더의 권한이 이 파일들을 보호합니다.

## 비용과 권한

- symphony가 시작하는 세션은 모두 Claude Code 세션이고, 다른 세션처럼 여러분의 Claude 요금제나 API 크레딧을 씁니다. 최대 사고 강도의 Opus는 그것을 빠르게 소진하고, 작업 하나는 세션 2~5개를 동시에 돌립니다.
- `bypassPermissions`는 모든 명령과 편집을 묻지 않고 실행합니다. 그러면 세션이 읽는 내용(파일, 웹 페이지, 도구 출력)이 여러분의 권한으로 명령을 실행하게 만들 수 있습니다. 이 모드를 고르면 새 직원 대화 상자와 새 작업 대화 상자에 빨간 경고가 나옵니다. 기본으로는 두 대화 상자 모두 여러분의 Claude Code 설정을 따릅니다.
- 페이지에서 시작한 세션에서는 Claude Code의 "이 폴더 신뢰" 질문에 bridge가 대신 답하므로, 믿을 수 있는 폴더에서만 세션을 시작하세요.
- "이 세션 동안 허용"은 세션이 끝날 때까지만 유효하며, Claude Code가 원래 묻던 것만 여러분의 권한 설정 범위 안에서 통과시킵니다. 다른 명령과 이어진 명령(`;`, `&&`, `|`, 리디렉션)은 절대 통과시키지 않고, 거부 규칙이나 여러분 조직의 설정을 넘어서지도 않습니다. 명령 규칙은 앞부분이 같은 모든 명령에 적용됩니다(`npm test`는 `npm test -- --watch`도 허용합니다).

## 보안

페이지에서 세션을 시작할 수 있다는 것은 곧 이 컴퓨터에서 명령을 실행할 수 있다는 뜻이므로, bridge는 누가 들어오는지 지킵니다.

- **페이지 키**: 사무실과 그 API를 쓰려면 페이지 키가 필요합니다. 페이지 키는 `symphony`가 여는 링크를 통해 브라우저가 받는 쿠키입니다. 키는 데이터 폴더의 `key` 파일로, 여러분만 읽을 수 있습니다. 프로그램은 `X-Symphony-Key` 헤더로 키를 제시합니다.
- **이 컴퓨터 전용**: bridge는 이 컴퓨터에서만 수신하고, 이 컴퓨터에서 온 연결만 받습니다. 다른 호스트를 향한 요청(DNS 리바인딩)과 다른 웹사이트가 여러분의 브라우저를 통해 보내는 쓰기 요청을 거부하며, 브라우저가 다른 사이트나 다른 포트에서 왔다고 알리는 요청에는 링크라 해도 API가 응답하지 않습니다. 다른 사이트는 이 페이지를 프레임 안에 넣을 수 없습니다. 페이지는 bridge 자신의 파일만 불러옵니다(Content Security Policy).
- **mod는 bridge를 확인한 뒤에** token을 보내고, `symphony` 프로그램도 bridge를 확인한 뒤에 페이지 키를 보여 주므로, bridge 주소에서 대신 수신하고 있는 다른 무언가는 둘 중 어느 것도 받지 못합니다.
- **막지 못하는 것**: 여러분 자신의 사용자 권한으로 실행되는 프로그램은 다른 파일과 마찬가지로 데이터 폴더의 페이지 키와 token을 읽을 수 있습니다. 또 브라우저는 한 사이트의 쿠키를 그 사이트의 모든 포트에 보냅니다. 이 컴퓨터의 다른 사용자가 `127.0.0.1`에서 돌리는 서버는 여러분의 브라우저가 그 서버를 열 때, 또는 bridge가 멈춰 있고 사무실이 아직 탭에 열려 있는 동안 그 서버가 bridge의 포트를 차지할 때 페이지 키 쿠키를 받게 됩니다. 다른 사람과 함께 쓰는 컴퓨터에서는 bridge를 멈추기 전에 사무실을 닫고, 키가 새어 나갔을 수 있다면 데이터 폴더의 `key` 파일을 지우세요(다음에 시작할 때 새 키가 만들어집니다).
- 프로그램에는 서명이 없습니다. 브라우저로 내려받은 사본은 macOS가 개발자를 확인할 수 없다며 막을 수 있습니다. "시스템 설정 → 개인정보 보호 및 보안"에서 "그래도 열기"를 누르거나, `xattr -d com.apple.quarantine symphony`를 한 번 실행하세요. Windows에서는 파일 탐색기의 "압축 풀기"로 푼 사본을 두 번 클릭하면 SmartScreen이 막을 수 있습니다. "추가 정보"를 누른 뒤 "실행"을 누르세요(설치 단계의 `Expand-Archive`는 다운로드 표시를 남기지 않습니다). 릴리스의 `checksums.txt`에는 모든 압축 파일의 SHA-256이 들어 있고, 모든 압축 파일에는 어디서 빌드됐는지에 대한 GitHub의 증명(attestation)이 붙어 있습니다. 설치 2단계에서 둘 다 검사합니다.

취약점을 알리려면 [SECURITY.md](SECURITY.md)를 보세요.

## 플랫폼

| 시스템 | 페이지에서 시작한 세션 | "터미널 열기" | 테스트 범위 |
| --- | --- | --- | --- |
| Linux | bridge 안의 의사 터미널 | 그래픽 터미널(x-terminal-emulator, gnome-terminal, konsole, xterm)에서 `symphony attach` 실행 | 엔드투엔드 테스트 전체 |
| macOS | bridge 안의 의사 터미널 | Terminal.app에서 `symphony attach` 실행 | 엔드투엔드 테스트 전체 |
| Windows | bridge 안의 의사 콘솔(ConPTY) | 새 콘솔 창에서 `symphony attach` 실행 | 단위 테스트와 tmux가 필요 없는 엔드투엔드 테스트, 실제 세션은 Windows 11에서 확인 |

"이 폴더 신뢰"는 모든 플랫폼에서 bridge가 대신 눌러 줍니다.

## 개발

```bash
go test -race ./...                         # bridge와 번역 단위 테스트
claude plugin test mods/symphony-bridge     # mod 단위 테스트
claude plugin validate .                    # 마켓플레이스 검사
claude plugin validate mods/symphony-bridge  # mod 검사
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # 엔드투엔드: 실제 세션을 띄우고 토큰을 소모합니다
```

- 기여: [CONTRIBUTING.md](CONTRIBUTING.md)에 개발 환경 준비, pull request 전에 돌릴 검사, 규칙이 있습니다. [CLAUDE.md](CLAUDE.md)는 코드의 자세한 지도입니다.
- mod 작업: `go run ./cmd/bridge -mod mods/symphony-bridge`로 실행하면 페이지에서 시작한 세션이 `--plugin-dir`로 여러분의 mod 사본을 받습니다. 직접 시작하는 세션에는 `--plugin-dir mods/symphony-bridge`를 손수 붙이세요.
- 페이지 작업: `go run ./cmd/bridge -web web`으로 실행하면 새로고침만으로 수정 내용이 보입니다.
- 실제 세션 없이 사무실 보기: 자기만의 데이터 폴더를 쓰는 두 번째 bridge를 띄워 가짜 활동으로 채웁니다. `D=$(mktemp -d)`와 `go run ./cmd/bridge -addr 127.0.0.1:7799 -data "$D"`를 실행한 다음, 다른 터미널에서(같은 `D`로) `go run ./cmd/demo -base http://127.0.0.1:7799 -data "$D"`를 실행하세요(demo는 그 데이터 폴더에서 bridge의 페이지 키를 읽습니다).
- 이 README의 스크린샷: `SYMPHONY_E2E=1 SYMPHONY_E2E_DOCS=1 go test -tags e2e -run TestDocsScreens ./e2e/`로 가짜 활동을 띄워 다시 찍습니다(토큰을 소모하지 않습니다).
- 번역: 언어마다 파일이 하나씩 있습니다. 페이지용은 `web/locales/<lang>.json`, bridge용은 `internal/i18n/locales/<lang>.json`이고 키는 `en.json`과 같습니다. mod의 몇 안 되는 메시지는 `mods/symphony-bridge/hooks/messages.ts`에 있습니다. 단위 테스트는 모든 언어에 모든 키와 같은 `{placeholders}`가 있는지 확인하고, 엔드투엔드 테스트 `TestI18n`은 모든 언어로 페이지를 열어 봅니다.
- 엔드투엔드 테스트에는 claude, tmux(테스트가 직접 움직이는 세션용), Chromium 또는 Chrome이 필요하므로 Linux와 macOS에서 돌아갑니다. `TestOffice`와 `TestModStartsBridge`는 실제 세션을 띄우고 토큰을 소모하며, 나머지는 소모하지 않습니다. 기본적으로 이 저장소의 mod를 쓰고, `SYMPHONY_E2E_MOD=installed`로 설정하면 설치된 mod를 씁니다. 스크린샷은 `e2e/artifacts/`에 저장됩니다.
- 릴리스: `mods/symphony-bridge/.claude-plugin/plugin.json`의 버전을 올리고, `CHANGELOG.md`에 `## vX.Y.Z (date)` 절을 추가하고, dev를 main에 반영한 다음, main의 그 커밋에 태그를 달아 push합니다(`git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`). GitHub Actions는 태그가 `plugin.json`의 버전과 같은지, 그 절이 있는지 확인하고, 모든 플랫폼용 프로그램을 빌드하고, 압축 파일이 어디서 빌드됐는지 증명(attestation)을 붙인 뒤, 그 절로 노트가 시작되는 릴리스를 게시합니다(`.github/workflows/release.yml`. `-rc.1` 같은 접미사를 붙이면 프리릴리스가 됩니다). 프리릴리스가 아니면 이어서 Homebrew formula와 Scoop manifest를 main에 커밋하므로(fast-forward가 되면 dev에도), 끝난 뒤에는 두 브랜치 모두 pull하세요. `.github/release.sh v1.0.0 dist`로 같은 압축 파일을 로컬에서 만들 수 있고, Linux에서는 바이트 하나까지 똑같습니다.

| 폴더 | 설명 |
| --- | --- |
| `mods/symphony-bridge/` | Claude Code mod (TypeScript) |
| `.claude-plugin/marketplace.json` | `claude plugin install`이 읽는 플러그인 마켓플레이스 |
| `cmd/bridge/`, `internal/bridge/` | bridge |
| `internal/i18n/` | 언어 감지와 bridge 번역 |
| `web/` | 사무실 페이지와 승인 페이지, 그리고 그 번역 (bridge에 내장됨) |
| `cmd/demo/` | 사무실에 보여 줄 가짜 활동 |
| `e2e/` | 엔드투엔드 테스트 |
| `docs/` | bridge와 페이지 사이의 프로토콜, 그리고 스크린샷 |

## 라이선스

[MIT](LICENSE)

symphony는 독립 프로젝트입니다. Anthropic과 제휴 관계가 없으며, Anthropic의 보증이나 후원을 받지 않습니다. Claude와 Claude Code는 Anthropic, PBC의 상표입니다.
