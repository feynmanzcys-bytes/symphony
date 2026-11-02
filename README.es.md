# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | **Español** | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

symphony es un proyecto independiente. No está afiliado a Anthropic ni cuenta con su respaldo ni su patrocinio. Claude y Claude Code son marcas comerciales de Anthropic, PBC.

Pon a trabajar a un equipo de sesiones de Claude Code en una oficina en pixel art: ve de un vistazo quién está trabajando y quién te está esperando, habla con cada uno en su propia ventana, y atiende allí las solicitudes de permiso y las preguntas de AskUserQuestion con un solo clic.

![La oficina pixelada: los empleados en sus puestos de la zona de trabajo, las pequeñas salas de reuniones a la izquierda, la sala de reuniones y la zona de descanso a la derecha](docs/office.png)

La página tiene temas: la oficina pixelada de arriba; la **consola HQ**, una consola de gestión con una franja de indicadores, la plantilla, una cola de aprobaciones, un tablero de proyectos y una cronología de actividad; **Compañeros**, las sesiones como compañeros en una app de mensajes, sus caras muestran qué hacen, la conversación en medio, el perfil y el ordenador a la derecha; y el **Control de misión**, la pared de una sala de control para una segunda pantalla: un panel de telemetría por sesión, las peticiones como alarmas, la actividad como rótulo. Elige uno en la barra superior; el bridge lo recuerda.

![La consola HQ: las cifras clave en una franja arriba, los empleados y su trabajo debajo](docs/hq.png)

![Compañeros: las sesiones en una lista a la izquierda con sus caras, una conversación en medio, el perfil y el ordenador a la derecha](docs/coworkers.png)

![Control de misión: una pared oscura con un panel de telemetría por sesión y un rótulo de actividad abajo](docs/mission.png)

## Instalación

La forma más rápida: abre Claude Code y pega

> Instálame https://github.com/feynmanzcys-bytes/symphony

Claude Code lee este README y hace los pasos de la sección siguiente; te pregunta antes de cualquier cambio que dure más allá de la instalación. Para instalarlo a mano, sigue los mismos pasos.

### Para agentes de IA: el procedimiento de instalación

Lees esto porque un usuario te ha pedido que instales symphony. Haz los pasos de abajo en orden, en una shell, y termina con el informe del paso 6. Toma tú las decisiones rutinarias (el archivo que descargar, la carpeta del `PATH`), pero pregunta al usuario antes de cualquier cambio que perdure más allá de esta instalación: iniciar symphony en cada inicio de sesión, editar un archivo rc de la shell o cambiar el `PATH` del usuario. Enumera todas las decisiones en el informe.

symphony tiene dos partes y necesita las dos:

1. **El programa `symphony`** (el bridge): un único binario, un servidor web local en `http://127.0.0.1:7788/` que sirve la página de la oficina y ejecuta las sesiones. Debería estar en el `PATH` del usuario, con el nombre `symphony`, porque el mod lo inicia con `symphony -background` cuando una sesión no encuentra ningún bridge en marcha.
2. **El plugin `symphony-bridge`** (el mod): un plugin de Claude Code del marketplace de este repositorio, instalado en el ámbito de usuario. Solo actúa en las sesiones iniciadas con `SYMPHONY_TOKEN` y no hace nada en las demás.

#### Paso 1: revisa la máquina

```bash
uname -sm                 # Linux/macOS: decide el archivo de abajo (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # hace falta Claude Code 2.1.x (probado con 2.1.316)
git --version             # solo para las tareas en equipo; si falta, anótalo en el informe, no te detengas
symphony -version         # ¿ya está instalado? muestra "symphony v1.1.1 (darwin/arm64)"
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7788/api/state   # ¿hay un bridge en marcha? 401: symphony 1.x; 200: uno más antiguo; 000: ninguno
claude plugin list --json # ¿el mod ya está instalado? busca una entrada con "id": "symphony-bridge@symphony"
brew --version            # ¿hay Homebrew (macOS, Linux)? el paso 2 puede instalar con él
```

En Windows usa PowerShell: `$env:PROCESSOR_ARCHITECTURE` es `AMD64` o `ARM64`, y `scoop --version` responde si Scoop está instalado.

Si falta `claude`, detente y dile al usuario que instale primero Claude Code. Si el programa y el mod ya están los dos, ve a «Actualizar», más abajo. Si solo está uno de los dos, haz los pasos del otro: los pasos 2 y 3 para el programa (salvo que `symphony -version` ya muestre la última versión), el paso 4 para el mod, y luego el paso 5. Un bridge que ya está en marcha sigue con el programa con el que se inició: si hay uno, lo detendrás antes del paso 5.

#### Paso 2: consigue el programa

**Con Homebrew o Scoop.** Este repositorio también es un tap de Homebrew y un bucket de Scoop. Si el paso 1 encontró Homebrew (macOS, Linux) o Scoop (Windows), puedes instalar con él en lugar del resto de este paso y del paso 3: descarga el archivo de la versión más reciente, lo comprueba con el SHA-256 de la fórmula o del manifiesto (escrito a partir del `checksums.txt` de la versión cuando esta salió) y pone `symphony` en el `PATH`. Luego sigue con el paso 4 e indica en el informe con cuál lo instalaste.

```bash
brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
brew install symphony
symphony -version          # muestra la versión y la plataforma
```

```powershell
scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony
scoop install symphony
symphony -version
```

Si no, descarga el archivo para esta máquina y el `checksums.txt` de la versión en una carpeta nueva solo para ellos, y comprueba el archivo antes de instalar nada. Los archivos son `symphony_<os>_<arch>.tar.gz`, con `<os>` `linux` o `darwin` (macOS) y `<arch>` `amd64` o `arm64`, y `symphony_windows_<arch>.zip`.

En Linux y macOS:

```bash
d=$(mktemp -d)   # una carpeta nueva que solo tú puedes leer
echo "$d"        # anótala: el paso 3 instala desde ella
cd "$d"
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/checksums.txt
if command -v sha256sum >/dev/null; then sha256sum -c --ignore-missing checksums.txt; else shasum -a 256 -c --ignore-missing checksums.txt; fi   # tiene que mostrar "symphony_<os>_<arch>.tar.gz: OK"
tar xzf symphony_<os>_<arch>.tar.gz              # el programa queda en "$d/symphony"
```

Si la comprobación no muestra `OK`, detente: no instales el programa y avisa al usuario. Con `gh` en lugar de curl, descarga en `$d` (gh crea la carpeta si todavía no existe) y luego haz allí la misma comprobación y el `tar`:

```bash
gh release download -R feynmanzcys-bytes/symphony -p 'symphony_<os>_<arch>.tar.gz' -p checksums.txt -D "$d"
```

En Windows, en PowerShell:

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$zip = "symphony_windows_$arch.zip"
$d = Join-Path $env:TEMP ("symphony-" + [guid]::NewGuid())   # una carpeta nueva
New-Item -ItemType Directory -Path $d | Out-Null
$url = 'https://github.com/feynmanzcys-bytes/symphony/releases/latest/download'
Invoke-WebRequest -UseBasicParsing -Uri "$url/$zip" -OutFile (Join-Path $d $zip)
Invoke-WebRequest -UseBasicParsing -Uri "$url/checksums.txt" -OutFile (Join-Path $d 'checksums.txt')
$want = ((Select-String -SimpleMatch -Path (Join-Path $d 'checksums.txt') -Pattern $zip).Line -split '\s+')[0]
$got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $d $zip)).Hash
if ($got -ne $want) { throw "checksum mismatch: $zip" } else { 'checksum OK' }
```

Después, si `gh` está instalado y con la sesión iniciada (`gh auth status` funciona), comprueba dónde se compiló el archivo: cada archivo de una versión lleva la atestación de GitHub de que lo compiló el workflow de publicación de este repositorio. Fuera de una terminal, `gh` no muestra nada cuando la comprobación pasa, así que guíate por su código de salida:

```bash
gh attestation verify "$d/symphony_<os>_<arch>.tar.gz" --repo feynmanzcys-bytes/symphony && echo 'attestation OK'
```

```powershell
gh attestation verify (Join-Path $d $zip) --repo feynmanzcys-bytes/symphony
if ($LASTEXITCODE -eq 0) { 'attestation OK' }
```

Las versiones anteriores a v1.2.0 no tienen atestación (`no attestations found`); para ellas basta la comprobación del SHA-256. Cualquier otro fallo: detente, igual que si falla la comprobación del SHA-256. Sin `gh`, o sin la sesión iniciada, sáltate esta comprobación y dilo en el informe.

**Desde el código fuente** en Linux o macOS, si el usuario lo prefiere (Go 1.27 o posterior). Compila el binario; `go run` no sirve, porque `-autostart` rechaza una compilación de `go run`:

```bash
d=$(mktemp -d)
git clone https://github.com/feynmanzcys-bytes/symphony.git "$d/src"
cd "$d/src" && go build -o "$d/symphony" ./cmd/bridge   # el programa queda en "$d/symphony", como desde un archivo
```

| Sistema | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Cada archivo contiene el programa (`symphony`, o `symphony.exe` en Windows), `LICENSE` y `README.md`. El `checksums.txt` de la versión tiene el SHA-256 de cada archivo.

#### Paso 3: ponlo en el PATH

Instala el programa del paso 2, `$d/symphony` (si tu shell no conserva las variables entre comandos, escribe la carpeta que mostró el paso 2). Usa una carpeta que ya esté en el `PATH` del usuario y en la que se pueda escribir sin `sudo`: `~/.local/bin`, `/usr/local/bin` o `/opt/homebrew/bin`, en ese orden.

```bash
install -m 755 "$d/symphony" ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: el binario no está firmado
symphony -version          # muestra "symphony v1.1.1 (linux/amd64)": la versión y la plataforma
```

Si ninguna de estas carpetas está en el `PATH`, pregunta al usuario antes de cambiar nada. Si dice que sí, crea `~/.local/bin`, añádela al archivo rc de la shell (`~/.zshrc` o `~/.bashrc`) e indícalo en el informe; si dice que no, instala el programa donde te diga y avísale de que el mod no puede iniciar el bridge por su cuenta mientras `symphony` no esté en el `PATH` de sus sesiones de Claude Code (el inicio al iniciar sesión, del paso 5, sigue funcionando).

En Windows, descomprímelo en una carpeta propia (`$d` y `$zip` vienen del paso 2; en una shell nueva, vuelve a definirlos):

```powershell
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Expand-Archive -LiteralPath (Join-Path $d $zip) -DestinationPath $dest -Force
& (Join-Path $dest 'symphony.exe') -version   # muestra "symphony v1.1.1 (windows/amd64)"
```

Añadir esa carpeta al `Path` del usuario es un cambio duradero: pregunta antes al usuario. Si dice que sí, ejecuta la línea de abajo y abre una shell nueva; si dice que no, usa la ruta completa. SmartScreen puede avisar en la primera ejecución: «Más información» y luego «Ejecutar de todas formas».

```powershell
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dest", 'User')
```

Si después de esto tu propia shell no encuentra `symphony` (leyó su `PATH` antes de que lo cambiaras), usa la ruta absoluta en el resto de los pasos.

#### Paso 4: instala el mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Esto añade el repositorio como el marketplace de plugins `symphony` e instala el mod en el ámbito de usuario, sin preguntas. En su lugar, un clon local del repositorio puede servir de marketplace: `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. Entonces las sesiones leen el mod de esa carpeta, y un `git pull` allí lo actualiza, así que guarda ese clon en un sitio duradero, no en una carpeta temporal.

Comprueba:

```bash
claude plugin list --json   # una entrada tiene "id": "symphony-bridge@symphony", "scope": "user", "enabled": true
```

- `"scope": "project"` o `"local"`: el mod solo se cargaría en una carpeta. Ejecuta `claude plugin uninstall symphony-bridge@symphony` y vuelve a instalarlo con `--scope user`.
- `"enabled": false`: `claude plugin enable symphony-bridge@symphony`.

#### Paso 5: inicia el bridge

Si el paso 1 encontró un bridge en marcha, detenlo primero, como en «Actualizar», más abajo: ejecuta el programa antiguo, y el nuevo solo lo encontraría ahí y se apartaría. Detenerlo termina las sesiones que inició, así que avisa antes al usuario.

Pregunta al usuario si symphony debe iniciarse cada vez que inicie sesión.

Si dice que sí:

```bash
symphony -autostart on -open=false
```

Esto inicia el bridge ahora y en cada inicio de sesión (un servicio de usuario de systemd en Linux, un agente de launchd en macOS, la clave Run del registro en Windows), y muestra, tras la fecha y la hora:

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/?key=<key>
```

Si dice que no:

```bash
symphony -background -open=false
```

Esto inicia el bridge separado de la terminal (no vuelve a iniciarse después de reiniciar la máquina) y muestra, en la salida de error estándar, tras la fecha y la hora:

```
the symphony bridge runs in the background at http://127.0.0.1:7788/?key=<key>; its log: <data folder>/bridge.log
```

Si ya hay en marcha un bridge de esta versión, muestra en su lugar (de la misma forma) `a symphony bridge is already running at http://127.0.0.1:7788/?key=<key>`; no pasa nada, no inicies otro. `<key>` es la clave de las páginas, que inicia la sesión de un navegador en la oficina: no la pegues en ningún otro sitio. Si el programa responde `127.0.0.1:7788 is taken: another program, or a symphony bridge with another data folder, answers there`, dile al usuario qué responde ahí (un bridge de symphony más antiguo también cuenta: detenlo como en «Actualizar») y no elijas otra dirección sin preguntar.

Comprueba toda la cadena con una sola petición, que presenta la clave de las páginas de la carpeta de datos (por la entrada estándar, para que la clave no aparezca en una línea de comandos que otros usuarios podrían ver):

```bash
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-${XDG_CONFIG_HOME:-$HOME/.config}/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options      # Linux
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-$HOME/Library/Application Support/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options   # macOS
```

```powershell
$data = if ($env:SYMPHONY_DATA) { $env:SYMPHONY_DATA } else { Join-Path $env:APPDATA 'symphony' }
(Invoke-WebRequest -UseBasicParsing -Headers @{ 'X-Symphony-Key' = (Get-Content (Join-Path $data 'key')) } -Uri http://127.0.0.1:7788/api/launch/options).Content
```

La carpeta de datos es `~/.config/symphony` en Linux (`$XDG_CONFIG_HOME/symphony` si esa variable está definida), `~/Library/Application Support/symphony` en macOS y `%AppData%\symphony` en Windows, o `SYMPHONY_DATA` si está definida.

La respuesta contiene `"enabled":true` y `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"1.1.1"}`: el bridge está en marcha, encontró `claude` y el mod está instalado donde las sesiones lo cargarán. Con `"enabled":false`, `mod.code` dice qué falla y `mod.fix`, el comando que lo arregla: `mod_not_installed` o `mod_project_scope` (vuelve al paso 4), `mod_disabled` (actívalo), `mod_check_failed` (`claude plugin list --json` falló, así que seguramente Claude Code es demasiado antiguo). `"mod":null` significa que el bridge no encontró `claude` en su `PATH` en absoluto (su registro lo dice): ponlo ahí, o inicia el bridge con `-claude /path/to/claude`. Una respuesta con `"code":"page_key"` significa que la petición no presentó ninguna clave o presentó una equivocada: comprueba que `cat` encontró el archivo de la clave (la carpeta de datos, más arriba); si lo encontró, el bridge en marcha se inició con otra carpeta de datos.

El registro del bridge es `bridge.log` en la carpeta de datos; si se inició al iniciar sesión, registra en `journalctl --user -u symphony` en Linux y en `~/Library/Logs/symphony.log` en macOS.

#### Paso 6: informa al usuario

Abre la oficina ejecutando `symphony`: con el bridge en marcha, abre la oficina en el navegador predeterminado, ya con la sesión iniciada, y sale. También muestra la dirección con `?key=`, que inicia la sesión en cualquier navegador de esta máquina.

Dile al usuario:

- Para abrir la oficina, ejecuta `symphony` (o haz doble clic en él); `http://127.0.0.1:7788/` a secas funciona en un navegador que ya tenga la sesión iniciada. Haz clic en el «+» de un puesto vacío para contratar al primer empleado: elige una carpeta y se inicia allí una sesión de Claude Code, dentro del bridge; su ventana en la página es su chat.
- Los empleados nuevos siguen la configuración de Claude Code del usuario (modelo, esfuerzo de razonamiento, modo de permisos) salvo que elija otros valores en el diálogo. `bypassPermissions` ejecuta cada comando y cada edición sin preguntar, y lo que lea una sesión puede entonces hacer que ejecute comandos en nombre del usuario; el diálogo avisa cuando se elige.
- Cada sesión gasta el plan de Claude o el crédito de API del usuario, como cualquier sesión de Claude Code; una tarea ejecuta de 2 a 5 sesiones a la vez.
- En la oficina solo aparecen las sesiones abiertas desde la página (o a mano con `SYMPHONY_TOKEN`, ver «Abrir sesiones» más abajo). Sus otras sesiones de Claude Code, incluida la que hace esta instalación, no se muestran.
- Qué se instaló y dónde: la ruta del programa, el mod en el ámbito de usuario, el bridge iniciado al iniciar sesión o en segundo plano, cualquier cambio en el `PATH` o en un archivo rc, las decisiones que tomaste, y que falta `git`, si falta.
- Cómo deshacerlo todo: «Desinstalar», más abajo.

Reglas: el bridge solo escucha en esta máquina (rechaza cualquier dirección que no sea de ella; deja la predeterminada, `127.0.0.1:7788`, salvo que el usuario elija otro puerto), no lo ejecutes con `sudo` y no inicies un segundo bridge en la misma dirección ni con la misma carpeta de datos (symphony se niega y dice cuál está en marcha).

#### Actualizar

Descarga la nueva versión como en el paso 2. El bridge en marcha sigue con el programa antiguo hasta que se reinicia (y en Windows no se puede sustituir un programa en ejecución), así que detenlo con `symphony -stop`, sustituye el programa como en el paso 3 y vuelve a iniciarlo tal como se ejecutaba antes: `symphony -autostart on -open=false` si se inicia al iniciar sesión (esto también lo inicia ya); si no, `symphony -background -open=false`. Si el bridge se ejecuta con un `-addr` o un `-data` distintos de los predeterminados, pasa los mismos a cada uno de estos comandos: `-stop` solo detiene el bridge de esa carpeta de datos (en `-addr`, o allí donde esté en marcha el bridge de esa carpeta), y dice `nothing to stop` cuando no responde ninguno. Detener el bridge termina las sesiones que inició, así que avisa antes al usuario. (`-stop` llegó con la v1.1.0; para detener un bridge v1.0.0, termina su proceso: `pkill -x symphony` en Linux y macOS, `Stop-Process -Name symphony` en Windows).

Si lo instalaste con Homebrew o Scoop, `brew update && brew upgrade symphony` (o `scoop update; scoop update symphony`) sustituye a la descarga y al paso 3, entre el mismo `-stop` y el nuevo arranque. Si symphony se inicia cada vez que el usuario inicia sesión, la entrada de ese inicio nombra el enlace de Homebrew al programa (`bin/symphony`), que la actualización mueve a la nueva versión; así que ahí `-autostart on` solo lo vuelve a iniciar ahora.

El mod se actualiza con `claude plugin update symphony-bridge@symphony` (con un clon como marketplace, haz `git pull` allí).

El programa y el mod comparten número de versión; [CHANGELOG.md](CHANGELOG.md) recoge qué cambió en cada versión.

#### Desinstalar

```bash
symphony -stop                                    # detiene el bridge y sus sesiones, sin importar cómo se haya iniciado (añade su -addr y su -data si no son los predeterminados)
symphony -autostart off                           # deja de iniciarlo al iniciar sesión (si no lo hacía, lo dice)
claude plugin uninstall symphony-bridge@symphony  # el mod
claude plugin marketplace remove symphony         # la entrada del marketplace
rm ~/.local/bin/symphony                          # el programa, donde lo haya puesto el paso 3
```

Si el paso 3 añadió `~/.local/bin` a un archivo rc de la shell, borra también esa línea. En Windows, en PowerShell:

```powershell
symphony -stop                                               # detiene el bridge y sus sesiones (añade su -addr y su -data si no son los predeterminados)
symphony -autostart off                                      # deja de iniciarlo al iniciar sesión
claude plugin uninstall symphony-bridge@symphony
claude plugin marketplace remove symphony
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Remove-Item -Recurse -Force $dest                            # el programa
$p = ([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -and $_ -ne $dest }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $p, 'User')    # la entrada de Path que añadió el paso 3
```

Si lo instalaste con Homebrew, ejecuta `brew uninstall symphony` y `brew untap feynmanzcys-bytes/symphony` en lugar de la línea de `rm`; con Scoop, `scoop uninstall symphony` y `scoop bucket rm symphony` en lugar de las cuatro últimas líneas de PowerShell.

La carpeta de datos (las cuentas con sus tokens, la clave de las páginas, el registro de mensajes, el libro de cuentas, las tareas, y archivos como `lock`, `bridge.addr` o un `*.corrupt-<time>` apartado) se queda; bórrala para eliminarlo todo. Al borrarla también se borra la clave de las páginas: el siguiente arranque crea una nueva y todos los navegadores tienen que volver a iniciar sesión. En Linux con un navegador snap o flatpak, `open.html` está en la carpeta propia de ese navegador (`~/snap/<snap>/common` o `~/.var/app/<app id>`); bórralo también.

## Qué hace

- **Cada sesión como un chat**: haz clic en un empleado y su ventana se abre en el centro de la página: arriba quién es, y debajo la sesión como un chat: las instrucciones, el razonamiento del modelo (plegado, «Pensando…» en naranja mientras dura), sus respuestas según llegan, las llamadas a herramientas y los mensajes de otras sesiones en morado. Escribe abajo y llega a la sesión como tu instrucción; el botón Detener la interrumpe.
- **Aprobaciones en el navegador**: las solicitudes de permiso (para ejecutar comandos o editar archivos) y las preguntas de AskUserQuestion suben como tarjetas sobre el cuadro de texto de la ventana de ese empleado, y el contador rojo de la barra superior te lleva a ellas. Si el bridge no está en marcha, no reconoce la cuenta o no es de confianza, la sesión vuelve a la terminal como siempre y nunca se queda colgada.
- **Una oficina en pixel art**: cada sesión es un empleado con su propio puesto fijo. El verde indica que está libre; el amarillo, que está trabajando; el rojo, que espera tu aprobación o tu respuesta, y el gris, que está fuera de servicio. Si no tienen nada que hacer, primero se quedan en su puesto con el teléfono o los videojuegos y luego se van de paseo: ven la tele en el sofá de la zona de descanso, juegan al futbolín o a pillar, charlan en la mesa redonda, se sirven un café o miran por la ventana; al rato se echan una siesta, casi siempre escondidos en la sala de reuniones. El tamaño del personaje depende del modelo, y sus músculos, del esfuerzo de razonamiento. La oficina es una planta del edificio de una gran empresa y llena la ventana: la zona de trabajo en el centro, pequeñas salas de reuniones para 3 a 5 personas con su placa a la izquierda, la sala de reuniones y la zona de descanso a la derecha, y ascensores por los que se entra y se sale. En la barra superior también eliges el tamaño del píxel, en cuatro pasos: la misma oficina, dibujada con píxeles más finos y más detalle en cada paso.
- **Encuentra a cualquiera y llámalo**: haz clic en un empleado libre y volverá a su puesto (y se quedará allí hasta su siguiente tarea); el buscador de la barra superior encuentra empleados por nombre, nombre de sesión o carpeta (pulsa `/` para ir a él), y un clic en un contador junto a él (libre, trabajando, pendiente, fuera de servicio) resalta a todos los que están en ese estado. La pestaña bajo la barra la recoge hacia arriba y la vuelve a bajar.
- **Mensajes entre sesiones**: cuando las sesiones se envían mensajes (SendMessage), un sobre cruza la oficina volando y el registro de mensajes los guarda.
- **Abre sesiones desde la página, sin que se abra nada**: haz clic en el «+» de un puesto vacío para contratar a alguien nuevo y elige la carpeta. El modelo, el esfuerzo de razonamiento y el modo de permisos siguen tu configuración de Claude Code («Predeterminado») salvo que elijas otros; al elegir `bypassPermissions` aparece un aviso en rojo de lo que permite. Haz clic en «A trabajar» en un empleado fuera de servicio para que vuelva a su mismo puesto, con las opciones con las que trabajó la última vez. La sesión corre en una terminal propia dentro del bridge, así que no se abre ninguna ventana ni se lleva el foco; «Abrir terminal» en la ventana del empleado muestra esa terminal cuando quieras, y cerrar la ventana no cambia nada.
- **Equipos en las salas de reuniones**: haz clic en una sala de reuniones pequeña libre, escribe una tarea y elige de 2 a 5 miembros: empleados, que reciben cada uno una segunda sesión solo para la tarea, o asesores de un solo uso. Por defecto el primer miembro dirige y reparte las partes con SendMessage; cada miembro trabaja en su propio worktree de git, en su propia rama, así que nadie pisa los archivos de nadie. Los miembros se quedan sentados en la sala hasta que termina la tarea.
- **10 idiomas**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) y Русский. La página usa el idioma de tu navegador y tiene un selector de idioma en la barra superior; los mensajes del bridge usan `-lang`, si no el idioma de tu sistema (`LANG` y similares), si no inglés; los del mod usan `SYMPHONY_LANG` (el idioma de la página, en una sesión abierta desde la página), si no el idioma de tu sistema, si no inglés.
- **Todo en local**: el bridge es un único programa en Go (solo con la biblioteca estándar; las pruebas de extremo a extremo usan además chromedp) que solo escucha en esta máquina, las páginas son JavaScript puro y nada depende de servicios de terceros.

![La ventana de un empleado: la sesión como un chat, con tarjetas de aprobación sobre su cuadro de texto](docs/approve.png)

## Requisitos

- Claude Code 2.1.316 (probado con esta versión). El mod usa los function hooks de Claude Code, que todavía son una API en acceso anticipado, así que ejecuta las pruebas de extremo a extremo después de actualizar Claude Code.
- El bridge viene como programa listo para usar en Linux, macOS y Windows; Go 1.27 o posterior solo hace falta para compilarlo desde el código fuente.
- `git`, solo para las tareas en equipo: cada miembro de una tarea trabaja en un worktree de git.
- Nada más: las sesiones abiertas desde la página corren dentro del bridge en todas las plataformas.

## Ejecutar el bridge

`symphony` inicia el bridge y abre la oficina en tu navegador, ya con la sesión iniciada (`-open=false` lo desactiva). El navegador recibe la clave de las páginas del bridge como cookie a través de `open.html`, un archivo de la carpeta de datos que solo tú puedes leer, así que la clave nunca aparece en una línea de comandos; en Linux, si tu navegador es un snap o un flatpak, ese archivo va a la carpeta propia del navegador, porque esos navegadores no pueden leer la carpeta de datos. Si abres <http://127.0.0.1:7788/> en un navegador sin la sesión iniciada, verás una página que explica cómo entrar: ejecuta `symphony` otra vez, o abre la dirección con `?key=` que el bridge escribió en su registro. <http://127.0.0.1:7788/approve> es una página más sencilla, solo para aprobaciones. Si inicias `symphony` otra vez mientras ya está en marcha, solo se abre la oficina. En Windows, con doble clic, `symphony.exe` se abre en su propia ventana, y al cerrarla se detiene el bridge.

| Opción | Predeterminado | Qué hace |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Dirección en la que escucha. Su host tiene que ser `127.0.0.1`, `localhost` o `::1` (como en `[::1]:7788`); el bridge rechaza cualquier otro |
| `-data` | `$SYMPHONY_DATA`; si no, `symphony` en tu carpeta de configuración | La carpeta de datos: las cuentas con sus tokens (`accounts.json`), la clave de las páginas (`key`) y `open.html`, el registro de mensajes (`messages.json`), el libro de cuentas (`ledger.json`), las tareas (`tasks.json`), el registro de `-background` (`bridge.log`), `lock` y `bridge.addr` (qué bridge la usa, y dónde), y un registro de mensajes o un libro de cuentas dañado, apartado como `*.corrupt-<time>` |
| `-claude` | `claude` | El ejecutable de claude |
| `-lang` | el idioma de tu sistema; si no, `en` | Idioma de los mensajes del bridge (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` o `LANG` fijan el predeterminado) |
| `-mod` | vacío | Para desarrollo: pasa a las sesiones el mod de esta carpeta con `--plugin-dir`, en lugar de la copia instalada |
| `-web` | vacío | Para desarrollo: sirve las páginas desde esta carpeta, así los cambios se ven al recargar |
| `-open` | `true` | Abre la oficina en el navegador cuando el bridge arranca; si el bridge de esta carpeta de datos ya está en marcha en `-addr`, solo abre su oficina |
| `-autostart` | — | `on`: inicia el bridge cada vez que inicias sesión, con las demás opciones dadas, y lo inicia ya; `off`: lo deshace |
| `-background` | — | Inicia el bridge en segundo plano, separado de la terminal y con su registro en `bridge.log` de la carpeta de datos, y vuelve cuando responde |
| `-stop` | — | Detiene el bridge de esta carpeta de datos (en `-addr` o, si no, allí donde esté en marcha), sin importar cómo se haya iniciado (sus sesiones terminan), y vuelve cuando ya no está; solo presenta la clave a un bridge que demuestre tenerla |
| `-version` | — | Muestra la versión y sale |

Si en `-addr` responde otra cosa (otro programa, un bridge con otra carpeta de datos o un bridge de symphony más antiguo), symphony lo dice y se detiene; solo presenta la clave de las páginas a un bridge que demuestre tenerla. Una carpeta de datos es de un solo bridge a la vez: si se inicia con una carpeta de datos cuyo bridge ya está en marcha en otra dirección, symphony dice dónde está, abre su oficina y se detiene (a un segundo bridge, dale su propio `-data`).

**Iniciarlo al iniciar sesión**: `symphony -autostart on` hace que el bridge se inicie cada vez que inicias sesión, con las opciones que pases junto (por ejemplo `-addr`), y lo inicia ya; `symphony -autostart off` lo deshace. En Linux es un servicio de usuario de systemd (`journalctl --user -u symphony` muestra su registro), en macOS un agente de launchd (registro en `~/Library/Logs/symphony.log`) y en Windows un valor bajo la clave Run del registro (el bridge corre en una ventana de consola; al cerrarla se detiene hasta el próximo inicio de sesión). `symphony -stop` detiene un bridge sin importar cómo se haya iniciado, hasta que se vuelva a iniciar (en el próximo inicio de sesión, si se inicia al iniciar sesión). Necesita el programa symphony en sí, no `go run`.

**Desde el código fuente**, con Go 1.27 o posterior: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Abrir sesiones

**Desde la página**: haz clic en el «+» de un puesto vacío, escribe un nombre y elige la carpeta y las opciones. El bridge ejecuta `claude` en una terminal propia, dentro del bridge: no se abre ninguna ventana. La ventana del empleado muestra la sesión como un chat, y su botón «Abrir terminal» abre una ventana de terminal conectada a la sesión (`symphony attach <id>` hace lo mismo desde cualquier terminal, con `-addr` y `-data` cuando el bridge no usa los valores predeterminados; cuando no se puede abrir ninguna ventana de terminal, la página muestra el comando exacto. `Ctrl-]` o cerrar la ventana desconecta, y la sesión sigue). Las sesiones son procesos hijos del bridge: detener el bridge las termina.

**Desde una terminal**: crea una cuenta para obtener un token, presentando la clave de las páginas, y luego inicia `claude` con el token:

```bash
printf 'X-Symphony-Key: %s\n' "$(cat ~/.config/symphony/key)" | curl -H @- -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # la respuesta incluye el token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

El archivo de la clave es `~/.config/symphony/key` en Linux, `~/Library/Application Support/symphony/key` en macOS y `%AppData%\symphony\key` en Windows, o `key` en la carpeta que indiquen `-data` / `SYMPHONY_DATA`.

Se recomienda el modo de permisos `manual`: en el modo `auto` la mayoría de las acciones no le preguntan a nadie, así que llegan pocas aprobaciones a la página. Si el bridge no está en la dirección predeterminada, define también `SYMPHONY_BRIDGE`: `http://127.0.0.1:<port>`, `http://localhost:<port>` o `http://[::1]:<port>` en esta máquina, o una dirección `https://`; con cualquier otra cosa el mod nunca contacta con el bridge. `SYMPHONY_LANG` elige el idioma de los mensajes del mod (si no, el idioma de tu sistema; si no, inglés).

Antes de que la sesión envíe su token a ningún sitio, el mod comprueba que el bridge de esa dirección conoce el token, sin enviarlo (`POST /api/hello`). Cuando el bridge no puede aceptar la sesión, la sesión muestra uno de estos mensajes, sus aprobaciones se quedan en la terminal y nunca se queda colgada:

| Mensaje | Qué significa |
| --- | --- |
| `no se puede conectar con el bridge` / `can't reach the bridge` | Nada responde en la dirección del bridge, e iniciar el bridge no sirvió (o la dirección no es de esta máquina). |
| `no se pudo iniciar el bridge (...): <reason>` / `couldn't start the bridge ...: <reason>` | Nada respondió, y `symphony -background` falló por el motivo indicado (por ejemplo, el bridge de esta carpeta de datos está en marcha en otra dirección: pon esa dirección en `SYMPHONY_BRIDGE`). |
| `el bridge no reconoce este SYMPHONY_TOKEN` / `the bridge doesn't know this SYMPHONY_TOKEN` | El bridge no tiene ninguna cuenta con este token: se borró, o el bridge usa otra carpeta de datos. |
| `no es un bridge de symphony en el que esta sesión pueda confiar` / `isn't a symphony bridge this session can trust` | Algo responde en la dirección pero no puede demostrar que conoce el token: otro programa, o un symphony más antiguo que el mod (actualízalo). El token no se envía. |
| `SYMPHONY_BRIDGE (...) debe estar en esta máquina o usar https` / `SYMPHONY_BRIDGE ... must be on this machine or use https` | La dirección no es una de las que acepta el mod; nunca se contacta con el bridge. |
| `esta cuenta ya tiene una sesión en línea` / `this account already has a session online` | Una cuenta sirve para una sola sesión a la vez; crea otra cuenta para esta sesión. |
| `esperando tu respuesta en la página de la oficina` / `waiting for your answer in the office page` | Una solicitud de permiso o una pregunta espera en la página de la oficina, no en la terminal; respóndela allí o pulsa Esc para cancelarla. Se muestra una vez por sesión. |

Si nada responde en la dirección del bridge en esta máquina, la sesión inicia el bridge ella misma con `symphony -background -open=false -addr <that address>` (si `symphony` está en tu PATH) y lo dice (`se inició el bridge (...) en segundo plano` / `started the bridge ... in the background`); el bridge sigue en marcha cuando la sesión termina. Define `SYMPHONY_DATA` si tus cuentas no están en la carpeta de datos predeterminada. Si el bridge de esta carpeta de datos ya está en marcha en otra dirección, lo dice y la sesión vuelve a la terminal: pon esa dirección en `SYMPHONY_BRIDGE`.

## Tareas en equipo

Una sala de reuniones pequeña es donde un equipo de 2 a 5 sesiones trabaja junto en una misma tarea. Las tareas necesitan `git`.

**Empezar una tarea**: haz clic en una sala de reuniones pequeña libre y describe lo que hay que hacer. Marca a los miembros en el orden que quieras (de 2 hasta las plazas de la sala), elige el modelo y el esfuerzo de razonamiento de cada uno y el modo de permisos del equipo (por defecto, la configuración propia de Claude Code; un empleado que ya ha trabajado empieza con su último modelo y esfuerzo; al elegir `bypassPermissions` aparece el aviso), y haz clic en «Empezar la tarea». Un miembro puede ser:

- **Un empleado**: recibe una segunda sesión solo para la tarea, que trabaja en la carpeta del empleado; su propia sesión, la de su puesto, se queda como estaba. Un empleado solo está en una tarea a la vez, y el que nunca ha trabajado todavía no tiene carpeta: haz clic antes una vez en «A trabajar».
- **Un asesor**: un participante de un solo uso creado para la tarea con los campos de siempre (nombre, carpeta, modelo, esfuerzo de razonamiento). No tiene puesto, sube en el ascensor y se va cuando termina la tarea.

**El líder**: por defecto el primer miembro dirige: divide la tarea, reparte las partes con SendMessage, revisa el trabajo, fusiona las ramas de los demás en la suya y te cuenta lo que se ha hecho. Desmarca la casilla del líder y cada uno recibe la tarea entera, y se la reparten entre ellos. En los dos casos, el bridge le da a cada miembro su primera instrucción: quién está en el equipo (los miembros se encuentran por su nombre con SendMessage), la rama de cada uno y las reglas.

**Git**: cada miembro trabaja en su propio worktree de git, en su propia rama de su carpeta, `symphony/<task id>/<member>` (por ejemplo `symphony/T1/bob`; un segundo miembro con el mismo nombre recibe `-2`), así que nadie pisa los archivos de nadie y tu copia de trabajo se queda como estaba. Los worktrees están en `.symphony/worktrees/` dentro de la carpeta, ocultos para git con `.git/info/exclude` (tu `.gitignore` no se toca). Cuando termina la tarea, los cambios que un miembro tenga sin commit se guardan con un commit en su rama, los archivos que git ignora en su worktree (resultados de compilación, `.env`, `node_modules` y similares) se mueven a `.symphony/kept/<member>-<task id>/` dentro de la carpeta, y los worktrees se eliminan. symphony nunca borra `.symphony/kept/`; bórralo tú cuando ya no necesites lo que contiene.

- En tus propios repositorios no se fusiona nada: las ramas se quedan para que las fusiones tú; si hay líder, su rama reúne el trabajo de todos.
- Una carpeta que no es un repositorio de git se convierte en uno para la tarea y lo sigue siendo: `git init` y luego un commit con todo lo que contiene, salvo los archivos que suelen guardar secretos o que ocupan mucho (`.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa*`, `id_ecdsa*`, `id_ed25519*`, `.npmrc`, `.netrc`, `.pypirc`, `node_modules/`, `.DS_Store`), que anota en `.git/info/exclude`. Esos archivos se quedan en la carpeta, pero no están en los worktrees de los miembros, así que el miembro que los necesite instala o crea los suyos. Cuando termina la tarea, las ramas de los miembros se fusionan en la rama de la carpeta, así que la carpeta tiene el resultado; el `.git` y las ramas se conservan. Si la tarea no llega a iniciarse, la carpeta vuelve a ser una carpeta normal.
- Si una fusión falla, se deshace y la base queda como estaba; las ramas se quedan con el trabajo hecho en ellas, y la ventana de la tarea dice qué rama tiene conflictos. Cualquier otra cosa que falle al terminar una tarea se queda como está, y la ventana de la tarea dice qué falló.

**En la oficina**: los miembros se sientan a la mesa de la sala durante toda la tarea, y mientras tanto el puesto de un empleado muestra una silla vacía. La placa de la sala muestra el id de la tarea; su luz, verde mientras la sala está libre, se pone amarilla mientras la tarea está en marcha y roja mientras un miembro espera tu aprobación o tu respuesta. Haz clic en la sala para abrir la ventana de la tarea: la tarea, los miembros con sus carpetas y ramas, el chat del equipo y «Terminar la tarea». Un clic en la figura de un miembro en la sala también abre la ventana de la tarea; un clic en un miembro dentro de ella abre su ventana de chat acoplada al lado, igual que la ventana de un empleado, un miembro a la vez (haz clic en otro miembro para cambiar). El contador rojo de la barra superior y el buscador abren a un miembro de una tarea de la misma forma.

**El chat del equipo**: la ventana de la tarea muestra los mensajes que los miembros se mandan entre sí, cada uno con la figura pixelada y el nombre de quien lo envía, y el texto empieza con `@` y el destinatario. Debajo hay un cuadro para que tú también le escribas al equipo: `@bob-T1 …` le llega solo a ese miembro (con varios `@names` les llega a todos ellos; al escribir `@` se abre una lista de los miembros para elegir con las flechas, Enter o un clic), y un mensaje sin `@` les llega a todos los de la tarea. Enter envía, Shift+Enter hace un salto de línea.

Una tarea termina cuando la terminas desde la ventana de la tarea o cuando termina su última sesión: el bridge detiene las sesiones de los miembros, se encarga de la parte de git como se explica arriba y borra a los asesores. Una tarea que sigue en marcha cuando se detiene el bridge se termina de la misma manera la próxima vez que el bridge arranca.

## Cuentas y puestos

- En symphony, una identidad es un nombre, un token y un puesto fijo. Una cuenta sirve para una sola sesión a la vez, más otra en la tarea en la que participa; cualquier otra sesión se rechaza y sus aprobaciones se quedan en la terminal.
- Aquí no se gestiona el inicio de sesión en Claude: todas las sesiones pueden compartir la misma cuenta de Claude.
- Los datos se guardan en `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) o `%AppData%\symphony` (Windows), o donde indiquen `-data` / `SYMPHONY_DATA`. `accounts.json` contiene los tokens y `key`, la clave de las páginas; solo su propietario puede leerlos (0600); en Windows los protegen los permisos de la carpeta del usuario.

## Coste y permisos

- Cada sesión que inicia symphony es una sesión de Claude Code y gasta tu plan de Claude o tu crédito de API como cualquier otra. Opus con el máximo esfuerzo lo agota rápido, y una tarea ejecuta de 2 a 5 sesiones a la vez.
- `bypassPermissions` ejecuta cada comando y cada edición sin preguntar. Lo que lea una sesión (un archivo, una página web, la salida de una herramienta) puede entonces hacer que ejecute comandos en tu nombre. Los diálogos de inicio y de tarea muestran un aviso en rojo cuando lo eliges; por defecto siguen tu configuración de Claude Code.
- En las sesiones abiertas desde la página, el bridge responde por ti a la pregunta «confiar en esta carpeta» de Claude Code, así que abre sesiones solo en carpetas de las que te fíes.

## Seguridad

La página puede abrir sesiones, lo que equivale a ejecutar comandos en esta máquina, así que el bridge vigila quién llega hasta él:

- **La clave de las páginas**: la oficina y su API necesitan la clave de las páginas, una cookie que tu navegador recibe a través del enlace que abre `symphony`. La clave es el archivo `key` de la carpeta de datos, que solo tú puedes leer; los programas la presentan en la cabecera `X-Symphony-Key`.
- **Solo esta máquina**: el bridge solo escucha en esta máquina y solo acepta conexiones de ella. Rechaza las peticiones dirigidas a cualquier otro host (DNS rebinding) y las escrituras que otros sitios web envíen a través de tu navegador, y su API no responde a ninguna petición que tu navegador indique que viene de otro sitio o de otro puerto, ni siquiera a un enlace. Otros sitios no pueden incrustar sus páginas en un marco. Las páginas solo cargan los archivos del propio bridge (una Content Security Policy).
- **El mod comprueba el bridge** antes de enviar su token, y el programa `symphony` lo comprueba antes de presentar la clave de las páginas, así que otra cosa que escuche en la dirección del bridge no recibe ninguno de los dos.
- **Contra qué no protege**: los programas que se ejecutan con tu propio usuario pueden leer la clave de las páginas y los tokens de la carpeta de datos, como cualquiera de tus archivos. Y los navegadores envían las cookies de un sitio a todos sus puertos: un servidor que otro usuario de esta máquina ejecute en `127.0.0.1` recibe la cookie de la clave de las páginas cuando tu navegador lo abre, o cuando ocupa el puerto del bridge mientras el bridge está detenido y la oficina sigue abierta en una pestaña. En una máquina compartida, cierra la oficina antes de detener el bridge y, si la clave puede haberse filtrado, borra el archivo `key` de la carpeta de datos (el siguiente arranque crea una nueva).
- Los programas no están firmados. macOS puede rechazar una copia descargada con el navegador porque no puede verificar al desarrollador: haz clic en «Abrir igualmente» en Ajustes del Sistema → Privacidad y seguridad, o ejecuta una vez `xattr -d com.apple.quarantine symphony`. SmartScreen de Windows también puede avisarte: haz clic en «Más información» y luego en «Ejecutar de todas formas». El `checksums.txt` de la versión tiene el SHA-256 de cada archivo; el paso 2 de la instalación lo comprueba.

Para informar de una vulnerabilidad, consulta [SECURITY.md](SECURITY.md).

## Plataformas

| Sistema | Sesiones abiertas desde la página | «Abrir terminal» | Pruebas |
| --- | --- | --- | --- |
| Linux | Una pseudoterminal dentro del bridge | Una terminal gráfica (x-terminal-emulator, gnome-terminal, konsole, xterm) que ejecuta `symphony attach` | Pruebas de extremo a extremo completas |
| macOS | Una pseudoterminal dentro del bridge | Terminal.app ejecutando `symphony attach` | Pruebas de extremo a extremo completas |
| Windows | Una pseudoconsola (ConPTY) dentro del bridge | Una ventana de consola nueva que ejecuta `symphony attach` | Solo compilación cruzada y pruebas unitarias |

El bridge acepta «confiar en esta carpeta» por ti en todas las plataformas.

## Desarrollo

```bash
go test -race ./...                         # pruebas unitarias del bridge y de las traducciones
claude plugin test mods/symphony-bridge     # pruebas unitarias del mod
claude plugin validate .                    # comprueba el marketplace
claude plugin validate mods/symphony-bridge  # comprueba el mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # de extremo a extremo: abre sesiones reales y gasta tokens
```

- Contribuir: [CONTRIBUTING.md](CONTRIBUTING.md) explica cómo preparar el entorno, las comprobaciones que hay que pasar antes de un pull request y las reglas; [CLAUDE.md](CLAUDE.md) es el mapa detallado del código.
- Para trabajar en el mod: ejecuta `go run ./cmd/bridge -mod mods/symphony-bridge` y las sesiones abiertas desde la página recibirán tu copia del mod con `--plugin-dir`; en las sesiones que abras a mano, añade `--plugin-dir mods/symphony-bridge` por tu cuenta.
- Para trabajar en las páginas: ejecuta `go run ./cmd/bridge -web web` y verás tus cambios al recargar.
- Para ver la oficina sin sesiones reales: inicia un segundo bridge con una carpeta de datos propia y llénalo de actividad inventada: `D=$(mktemp -d)`, `go run ./cmd/bridge -addr 127.0.0.1:7799 -data "$D"` y luego, en otra terminal (con el mismo valor de `D`), `go run ./cmd/demo -base http://127.0.0.1:7799 -data "$D"` (la demo lee la clave de las páginas del bridge en esa carpeta de datos).
- Las capturas de pantalla de este README: `SYMPHONY_E2E=1 SYMPHONY_E2E_DOCS=1 go test -tags e2e -run TestDocsScreens ./e2e/` las vuelve a hacer con actividad inventada (sin gastar tokens).
- Traducciones: cada idioma es un archivo, `web/locales/<lang>.json` para las páginas y `internal/i18n/locales/<lang>.json` para el bridge, con las mismas claves que `en.json`; los pocos mensajes del mod están en `mods/symphony-bridge/hooks/messages.ts`. Las pruebas unitarias comprueban que cada idioma tenga todas las claves y los mismos `{placeholders}`, y la prueba de extremo a extremo `TestI18n` abre las páginas en todos los idiomas.
- Las pruebas de extremo a extremo necesitan claude, tmux (para las sesiones que las propias pruebas manejan) y Chromium o Chrome, así que se ejecutan en Linux y macOS. `TestOffice` y `TestModStartsBridge` abren sesiones reales y gastan tokens; las demás no. Por defecto usan el mod de este repositorio; define `SYMPHONY_E2E_MOD=installed` para usar el mod instalado. Las capturas de pantalla se guardan en `e2e/artifacts/`.
- Versiones: sube la versión en `mods/symphony-bridge/.claude-plugin/plugin.json`, añade una sección `## vX.Y.Z (date)` a `CHANGELOG.md`, lleva dev a main, luego etiqueta ese commit en main y sube la etiqueta (`git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`); GitHub Actions comprueba que la etiqueta sea la versión de `plugin.json` y que la sección exista, compila el programa para todas las plataformas y publica una versión cuyas notas empiezan con esa sección (`.github/workflows/release.yml`; un sufijo como `-rc.1` la convierte en preversión). `.github/release.sh v1.0.0 dist` genera los mismos archivos en local.

| Carpeta | Qué es |
| --- | --- |
| `mods/symphony-bridge/` | El mod de Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | El marketplace de plugins que lee `claude plugin install` |
| `cmd/bridge/`, `internal/bridge/` | El bridge |
| `internal/i18n/` | La detección del idioma y las traducciones del bridge |
| `web/` | Las páginas de la oficina y de aprobaciones, con sus traducciones, integradas en el bridge |
| `cmd/demo/` | Actividad inventada para la oficina |
| `e2e/` | Pruebas de extremo a extremo |
| `docs/` | El protocolo entre el bridge y las páginas, y las capturas de pantalla |

## Licencia

[MIT](LICENSE)

symphony es un proyecto independiente. No está afiliado a Anthropic ni cuenta con su respaldo ni su patrocinio. Claude y Claude Code son marcas comerciales de Anthropic, PBC.
