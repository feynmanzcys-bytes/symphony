# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | **Español** | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Pon a trabajar a un equipo de sesiones de Claude Code en una oficina en pixel art: ve de un vistazo quién está trabajando y quién te está esperando, habla con cada uno en su propia ventana, y atiende allí las solicitudes de permiso y las preguntas de AskUserQuestion con un solo clic.

![La oficina](docs/office.png)

La página tiene temas: la oficina pixelada de arriba y la **consola HQ**, una consola de gestión con una franja de indicadores, la plantilla, una cola de aprobaciones, un tablero de proyectos y una cronología de actividad. Elige uno en la barra superior; el bridge lo recuerda.

![La consola HQ](docs/hq.png)

## Instalación

La forma más rápida: abre Claude Code y pega

> Instálame https://github.com/feynmanzcys-bytes/symphony

Claude Code lee este README y hace todo lo que se describe en la sección siguiente. Para instalarlo a mano, sigue los mismos pasos.

### Para agentes de IA: el procedimiento de instalación

Lees esto porque un usuario te ha pedido que instales symphony. Haz los pasos de abajo en orden, en una shell, y termina con el informe del paso 6. No le preguntes nada al usuario salvo que un paso lo diga; toma tú las decisiones rutinarias y enuméralas en el informe.

symphony tiene dos partes y necesita las dos:

1. **El programa `symphony`** (el bridge): un único binario, un servidor web local en `http://127.0.0.1:7788/` que sirve la página de la oficina y ejecuta las sesiones. Tiene que estar en el `PATH` del usuario, con el nombre `symphony`, porque el mod lo inicia con `symphony -background` cuando una sesión no encuentra ningún bridge en marcha.
2. **El plugin `symphony-bridge`** (el mod): un plugin de Claude Code del marketplace de este repositorio, instalado en el ámbito de usuario. Solo actúa en las sesiones iniciadas con `SYMPHONY_TOKEN` y no hace nada en las demás.

#### Paso 1: revisa la máquina

```bash
uname -sm                 # Linux/macOS: decide el archivo de abajo (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # hace falta Claude Code 2.1.x (probado con 2.1.316)
git --version             # solo para las tareas en equipo; si falta, anótalo en el informe, no te detengas
symphony -version         # ¿ya está instalado? muestra "symphony v0.10.0 (darwin/arm64)"
curl -s http://127.0.0.1:7788/api/state | head -c 40   # ¿ya hay un bridge en marcha? muestra {"type":"snapshot",...
claude plugin list --json # ¿el mod ya está instalado? busca "id":"symphony-bridge@symphony"
```

En Windows usa PowerShell: `$env:PROCESSOR_ARCHITECTURE` es `AMD64` o `ARM64`, y `Invoke-WebRequest http://127.0.0.1:7788/api/state` en lugar de curl.

Si falta `claude`, detente y dile al usuario que instale primero Claude Code. Si el programa y el mod ya están los dos, ve a «Actualizar», más abajo.

#### Paso 2: consigue el programa

Usa la primera opción que funcione:

- **A. Con `gh`** (funciona mientras este repositorio sea privado, siempre que `gh auth status` tenga una sesión iniciada):
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. Con `curl`** (solo cuando el repositorio sea público; un 404 significa que uses A o C):
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. Desde el código fuente** (Go 1.27 o posterior; el clon se reutiliza en el paso 4):
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  Compila el binario; `go run` no sirve, porque `-autostart` rechaza una compilación de `go run`.

| Sistema | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Cada archivo contiene el programa (`symphony`, o `symphony.exe` en Windows), `LICENSE` y `README.md`. El `checksums.txt` de la versión tiene el SHA-256 de cada archivo (`sha256sum -c --ignore-missing checksums.txt`, o `shasum -a 256 -c` en macOS).

#### Paso 3: ponlo en el PATH

Elige una carpeta que ya esté en el `PATH` del usuario y en la que se pueda escribir sin `sudo`: `~/.local/bin`, `/usr/local/bin` o `/opt/homebrew/bin`, en ese orden. Si no existe ninguna, crea `~/.local/bin`, añádela al archivo rc de la shell (`~/.zshrc` o `~/.bashrc`) e indícalo en el informe.

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: el binario no está firmado
symphony -version          # muestra "symphony v0.10.0 (linux/amd64)": la versión y la plataforma
```

En Windows: `Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`, luego añade esa carpeta al `Path` del usuario (`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`) y abre una shell nueva. SmartScreen puede avisar en la primera ejecución: «Más información» y luego «Ejecutar de todas formas».

Si después de esto tu propia shell no encuentra `symphony` (leyó su `PATH` antes de que lo cambiaras), usa la ruta absoluta en el resto de los pasos.

#### Paso 4: instala el mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Esto añade el repositorio como el marketplace de plugins `symphony` e instala el mod en el ámbito de usuario, sin preguntas. Clona el repositorio con git, así que mientras el repositorio sea privado necesita acceso por git a GitHub; si falla por eso, usa un clon como marketplace (el del paso 2 C, o haz uno): `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. Con un clon como marketplace, las sesiones leen el mod de esa carpeta, y un `git pull` allí lo actualiza.

Comprueba:

```bash
claude plugin list --json   # una entrada tiene "id":"symphony-bridge@symphony", "scope":"user", "enabled":true
```

- `"scope":"project"` o `"local"`: el mod solo se cargaría en una carpeta. Ejecuta `claude plugin uninstall symphony-bridge@symphony` y vuelve a instalarlo con `--scope user`.
- `"enabled":false`: `claude plugin enable symphony-bridge@symphony`.

#### Paso 5: inicia el bridge

```bash
symphony -autostart on -open=false
```

Esto inicia el bridge ahora y en cada inicio de sesión (un servicio de usuario de systemd en Linux, un agente de launchd en macOS, la clave Run del registro en Windows), y muestra

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

Si el usuario dijo que no quiere que se inicie al iniciar sesión, ejecuta en su lugar `symphony -background -open=false`: inicia el bridge separado de la terminal y muestra `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log`. Los dos comandos responden `a symphony bridge is already running at http://127.0.0.1:7788/` cuando ya hay uno; no pasa nada, no inicies otro.

Comprueba toda la cadena con una sola petición:

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

La respuesta contiene `"enabled":true` y `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.10.0"}`: el bridge está en marcha, encontró `claude` y el mod está instalado donde las sesiones lo cargarán. Con `"enabled":false`, `mod.code` dice qué falla y `mod.fix`, el comando que lo arregla: `mod_not_installed` o `mod_project_scope` (vuelve al paso 4), `mod_disabled` (actívalo), `mod_check_failed` (`claude plugin list --json` falló, así que seguramente Claude Code es demasiado antiguo). `"mod":null` significa que el bridge no encontró `claude` en su `PATH` (su registro lo dice): ponlo ahí, o inicia el bridge con `-claude /path/to/claude`.

El registro del bridge es `bridge.log` en la carpeta de datos (`~/.config/symphony` en Linux, `~/Library/Application Support/symphony` en macOS, `%AppData%\symphony` en Windows); si se inició al iniciar sesión, registra en `journalctl --user -u symphony` en Linux y en `~/Library/Logs/symphony.log` en macOS.

#### Paso 6: informa al usuario

Abre la oficina en el navegador (`open http://127.0.0.1:7788/` en macOS, `xdg-open` en Linux, `start` en Windows) y dile al usuario:

- La oficina está en `http://127.0.0.1:7788/`. Haz clic en el «+» de un puesto vacío para contratar al primer empleado: elige una carpeta y se inicia allí una sesión de Claude Code, dentro del bridge; su ventana en la página es su chat.
- En la oficina solo aparecen las sesiones abiertas desde la página (o a mano con `SYMPHONY_TOKEN`, ver «Abrir sesiones» más abajo). Sus otras sesiones de Claude Code, incluida la que hace esta instalación, no se muestran.
- Qué se instaló y dónde: la ruta del programa, el mod en el ámbito de usuario, el bridge iniciado al iniciar sesión o en segundo plano, cualquier cambio en el `PATH`, y que falta `git`, si falta.
- Cómo deshacerlo todo: «Desinstalar», más abajo.

Reglas: no pongas el bridge a escuchar en nada que no sea esta máquina (`-addr` se queda en `127.0.0.1:7788`; la página no tiene inicio de sesión), no lo ejecutes con `sudo` y no inicies un segundo bridge en la misma dirección.

#### Actualizar

Descarga la nueva versión como en el paso 2 y sustituye el programa como en el paso 3; el mod se actualiza con `claude plugin update symphony-bridge@symphony` (con un clon como marketplace, haz `git pull` allí). El bridge en marcha sigue con el programa antiguo hasta que se reinicia: `symphony -autostart off` y luego `symphony -autostart on -open=false` (o detenlo e inícialo como se inició). Detener el bridge termina las sesiones que inició, así que avisa antes al usuario. El programa y el mod comparten número de versión; [CHANGELOG.md](CHANGELOG.md) recoge qué cambió en cada versión.

#### Desinstalar

```bash
symphony -autostart off                           # detiene el bridge y sus sesiones, y deja de iniciarlo al iniciar sesión
claude plugin uninstall symphony-bridge@symphony  # el mod
claude plugin marketplace remove symphony         # la entrada del marketplace
rm ~/.local/bin/symphony                          # el programa, donde lo haya puesto el paso 3
```

La carpeta de datos (las cuentas con sus tokens, el registro de mensajes, las tareas) se queda; bórrala para eliminarlo todo.

## Qué hace

- **Cada sesión como un chat**: haz clic en un empleado y su ventana se abre en el centro de la página: arriba quién es, y debajo la sesión como un chat: las instrucciones, el razonamiento del modelo (plegado, «Pensando…» en naranja mientras dura), sus respuestas según llegan, las llamadas a herramientas y los mensajes de otras sesiones en morado. Escribe abajo y llega a la sesión como tu instrucción; el botón Detener la interrumpe.
- **Aprobaciones en el navegador**: las solicitudes de permiso (para ejecutar comandos o editar archivos) y las preguntas de AskUserQuestion suben como tarjetas sobre el cuadro de texto de la ventana de ese empleado, y el contador rojo de la barra superior te lleva a ellas. Si el bridge no está en marcha o no reconoce la cuenta, la sesión vuelve a la terminal como siempre y nunca se queda colgada.
- **Una oficina en pixel art**: cada sesión es un empleado con su propio puesto fijo. El verde indica que está libre; el amarillo, que está trabajando; el rojo, que espera tu aprobación o tu respuesta, y el gris, que está fuera de servicio. Si no tienen nada que hacer, primero se quedan en su puesto con el teléfono o los videojuegos y luego se van de paseo: ven la tele en el sofá de la zona de descanso, juegan al futbolín o a pillar, charlan en la mesa redonda, se sirven un café o miran por la ventana; al rato se echan una siesta, casi siempre escondidos en la sala de reuniones. El tamaño del personaje depende del modelo, y sus músculos, del esfuerzo de razonamiento. La oficina es una planta del edificio de una gran empresa y llena la ventana: la zona de trabajo en el centro, pequeñas salas de reuniones para 3 a 5 personas con su placa a la izquierda, la sala de reuniones y la zona de descanso a la derecha, y ascensores por los que se entra y se sale. En la barra superior también eliges el tamaño del píxel, en cuatro pasos: la misma oficina, dibujada con píxeles más finos y más detalle en cada paso.
- **Encuentra a cualquiera y llámalo**: haz clic en un empleado libre y volverá a su puesto (y se quedará allí hasta su siguiente tarea); el buscador de la barra superior encuentra empleados por nombre, nombre de sesión o carpeta (pulsa `/` para ir a él), y un clic en un contador junto a él (libre, trabajando, pendiente, fuera de servicio) resalta a todos los que están en ese estado. La pestaña bajo la barra la recoge hacia arriba y la vuelve a bajar.
- **Mensajes entre sesiones**: cuando las sesiones se envían mensajes (SendMessage), un sobre cruza la oficina volando y el registro de mensajes los guarda.
- **Abre sesiones desde la página, sin que se abra nada**: haz clic en el «+» de un puesto vacío para contratar a alguien nuevo y elige la carpeta, el modelo, el esfuerzo de razonamiento y el modo de permisos (un empleado nuevo empieza con todo permitido, Opus y el máximo razonamiento); haz clic en «A trabajar» en un empleado fuera de servicio para que vuelva a su mismo puesto. La sesión corre en una terminal propia dentro del bridge, así que no se abre ninguna ventana ni se lleva el foco; «Abrir terminal» en la ventana del empleado muestra esa terminal cuando quieras, y cerrar la ventana no cambia nada.
- **Equipos en las salas de reuniones**: haz clic en una sala de reuniones pequeña libre, escribe una tarea y elige de 2 a 5 miembros: empleados, que reciben cada uno una segunda sesión solo para la tarea, o asesores de un solo uso. Por defecto el primer miembro dirige y reparte las partes con SendMessage; cada miembro trabaja en su propio worktree de git, en su propia rama, así que nadie pisa los archivos de nadie. Los miembros se quedan sentados en la sala hasta que termina la tarea.
- **10 idiomas**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) y Русский. La página usa el idioma de tu navegador y tiene un selector de idioma en la barra superior; los mensajes del bridge usan `-lang`, si no el idioma de tu sistema (`LANG` y similares), si no inglés; los del mod usan `SYMPHONY_LANG` (el idioma de la página, en una sesión abierta desde la página), si no el idioma de tu sistema, si no inglés.
- **Todo en local**: el bridge es un único programa en Go (solo con la biblioteca estándar; las pruebas de extremo a extremo usan además chromedp), las páginas son JavaScript puro y nada depende de servicios de terceros.

![Aprobaciones en el navegador](docs/approve.png)

## Requisitos

- Claude Code 2.1.316 (probado con esta versión). El mod usa los function hooks de Claude Code, que todavía son una API en acceso anticipado, así que ejecuta las pruebas de extremo a extremo después de actualizar Claude Code.
- El bridge viene como programa listo para usar en Linux, macOS y Windows; Go 1.27 o posterior solo hace falta para compilarlo desde el código fuente.
- `git`, solo para las tareas en equipo: cada miembro de una tarea trabaja en un worktree de git.
- Nada más: las sesiones abiertas desde la página corren dentro del bridge en todas las plataformas.

## Ejecutar el bridge

`symphony` inicia el bridge y abre la oficina en tu navegador en <http://127.0.0.1:7788/> (`-open=false` lo desactiva); <http://127.0.0.1:7788/approve> es una página más sencilla, solo para aprobaciones. Si inicias el bridge otra vez mientras ya está en marcha, solo se abre la oficina. En Windows, con doble clic, `symphony.exe` se abre en su propia ventana, y al cerrarla se detiene el bridge.

| Opción | Predeterminado | Qué hace |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Dirección en la que escucha. La página todavía no tiene inicio de sesión, así que no uses una dirección que no sea de esta máquina |
| `-data` | `$SYMPHONY_DATA`; si no, `symphony` en tu carpeta de configuración | La carpeta de datos: las cuentas con sus tokens (`accounts.json`), el registro de mensajes (`messages.json`), las tareas (`tasks.json`) y el registro de `-background` (`bridge.log`) |
| `-claude` | `claude` | El ejecutable de claude |
| `-lang` | el idioma de tu sistema; si no, `en` | Idioma de los mensajes del bridge (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` o `LANG` fijan el predeterminado) |
| `-mod` | vacío | Para desarrollo: pasa a las sesiones el mod de esta carpeta con `--plugin-dir`, en lugar de la copia instalada |
| `-web` | vacío | Para desarrollo: sirve las páginas desde esta carpeta, así los cambios se ven al recargar |
| `-open` | `true` | Abre la oficina en el navegador cuando el bridge arranca; si ya hay un bridge en `-addr`, solo abre su oficina |
| `-autostart` | — | `on`: inicia el bridge cada vez que inicias sesión, con las demás opciones dadas, y lo inicia ya; `off`: lo deshace |
| `-background` | — | Inicia el bridge en segundo plano, separado de la terminal y con su registro en `bridge.log` de la carpeta de datos, y vuelve cuando responde |
| `-version` | — | Muestra la versión y sale |

**Iniciarlo al iniciar sesión**: `symphony -autostart on` hace que el bridge se inicie cada vez que inicias sesión, con las opciones que pases junto (por ejemplo `-addr`), y lo inicia ya; `symphony -autostart off` lo deshace. En Linux es un servicio de usuario de systemd (`journalctl --user -u symphony` muestra su registro), en macOS un agente de launchd (registro en `~/Library/Logs/symphony.log`) y en Windows un valor bajo la clave Run del registro (el bridge corre en una ventana de consola; al cerrarla se detiene hasta el próximo inicio de sesión). Necesita el programa symphony en sí, no `go run`.

**Desde el código fuente**, con Go 1.27 o posterior: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Abrir sesiones

**Desde la página**: haz clic en el «+» de un puesto vacío, escribe un nombre y elige la carpeta y las opciones. El bridge ejecuta `claude` en una terminal propia, dentro del bridge: no se abre ninguna ventana. La ventana del empleado muestra la sesión como un chat, y su botón «Abrir terminal» abre una ventana de terminal conectada a la sesión (`symphony attach <id>` hace lo mismo desde cualquier terminal; `Ctrl-]` o cerrar la ventana desconecta, y la sesión sigue). Las sesiones son procesos hijos del bridge: detener el bridge las termina.

**Desde una terminal**: crea una cuenta para obtener un token y luego inicia `claude` con él:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # la respuesta incluye el token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Se recomienda el modo de permisos `manual`: en el modo `auto` la mayoría de las acciones no le preguntan a nadie, así que llegan pocas aprobaciones a la página. Si el bridge no está en la dirección predeterminada, define también `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` elige el idioma de los mensajes del mod (si no, el idioma de tu sistema; si no, inglés).

Si nada responde en la dirección del bridge en esta máquina, la sesión inicia el bridge ella misma con `symphony -background -open=false -addr <that address>` (si `symphony` está en tu PATH), y el bridge sigue en marcha cuando la sesión termina. Define `SYMPHONY_DATA` si tus cuentas no están en la carpeta de datos predeterminada.

## Tareas en equipo

Una sala de reuniones pequeña es donde un equipo de 2 a 5 sesiones trabaja junto en una misma tarea. Las tareas necesitan `git` y el mod en la versión 0.7.0 (`claude plugin update symphony-bridge@symphony`).

**Empezar una tarea**: haz clic en una sala de reuniones pequeña libre y describe lo que hay que hacer. Marca a los miembros en el orden que quieras (de 2 hasta las plazas de la sala), elige el modelo y el esfuerzo de razonamiento de cada uno y el modo de permisos del equipo (todo permitido por defecto), y haz clic en «Empezar la tarea». Un miembro puede ser:

- **Un empleado**: recibe una segunda sesión solo para la tarea, que trabaja en la carpeta del empleado; su propia sesión, la de su puesto, se queda como estaba. Un empleado solo está en una tarea a la vez, y el que nunca ha trabajado todavía no tiene carpeta: haz clic antes una vez en «A trabajar».
- **Un asesor**: un participante de un solo uso creado para la tarea con los campos de siempre (nombre, carpeta, modelo, esfuerzo de razonamiento). No tiene puesto, sube en el ascensor y se va cuando termina la tarea.

**El líder**: por defecto el primer miembro dirige: divide la tarea, reparte las partes con SendMessage, revisa el trabajo, fusiona las ramas de los demás en la suya y te cuenta lo que se ha hecho. Desmarca la casilla del líder y cada uno recibe la tarea entera, y se la reparten entre ellos. En los dos casos, el bridge le da a cada miembro su primera instrucción: quién está en el equipo (los miembros se encuentran por su nombre con SendMessage), la rama de cada uno y las reglas.

**Git**: cada miembro trabaja en su propio worktree de git, en su propia rama de su carpeta, `symphony/<task id>/<member>` (por ejemplo `symphony/T1/bob`; un segundo miembro con el mismo nombre recibe `-2`), así que nadie pisa los archivos de nadie y tu copia de trabajo se queda como estaba. Los worktrees están en `.symphony/worktrees/` dentro de la carpeta, ocultos para git con `.git/info/exclude` (tu `.gitignore` no se toca). Cuando termina la tarea, los cambios que un miembro tenga sin commit se guardan con un commit en su rama, así que no se pierde nada, y los worktrees se eliminan.

- En tus propios repositorios no se fusiona nada: las ramas se quedan para que las fusiones tú; si hay líder, su rama reúne el trabajo de todos.
- Una carpeta que no es un repositorio de git se convierte en uno para la tarea (`git init` y un commit con todo lo que contiene) y lo sigue siendo. Cuando termina la tarea, las ramas de los miembros se fusionan en ella, así que la carpeta tiene el resultado; el `.git` y las ramas se conservan.
- Si una fusión falla, se deshace y la base queda como estaba; las ramas se quedan con el trabajo hecho en ellas, y la ventana de la tarea dice qué rama tiene conflictos.

**En la oficina**: los miembros se sientan a la mesa de la sala durante toda la tarea, y mientras tanto el puesto de un empleado muestra una silla vacía. La placa de la sala muestra el id de la tarea; su luz, verde mientras la sala está libre, se pone amarilla mientras la tarea está en marcha y roja mientras un miembro espera tu aprobación o tu respuesta. Haz clic en la sala para abrir la ventana de la tarea: la tarea, los miembros con sus carpetas y ramas, el chat del equipo y «Terminar la tarea». Un clic en la figura de un miembro en la sala también abre la ventana de la tarea; un clic en un miembro dentro de ella abre su ventana de chat acoplada al lado, igual que la ventana de un empleado, un miembro a la vez (haz clic en otro miembro para cambiar). El contador rojo de la barra superior y el buscador abren a un miembro de una tarea de la misma forma.

**El chat del equipo**: la ventana de la tarea muestra los mensajes que los miembros se mandan entre sí, cada uno con la figura pixelada y el nombre de quien lo envía, y el texto empieza con `@` y el destinatario. Debajo hay un cuadro para que tú también le escribas al equipo: `@bob-T1 …` le llega solo a ese miembro (con varios `@names` les llega a todos ellos; al escribir `@` se abre una lista de los miembros para elegir con las flechas, Enter o un clic), y un mensaje sin `@` les llega a todos los de la tarea. Enter envía, Shift+Enter hace un salto de línea.

Una tarea termina cuando la terminas desde la ventana de la tarea o cuando termina su última sesión: el bridge detiene las sesiones de los miembros, se encarga de la parte de git como se explica arriba y borra a los asesores. Una tarea que sigue en marcha cuando se detiene el bridge se termina de la misma manera la próxima vez que el bridge arranca.

## Cuentas y puestos

- En symphony, una identidad es un nombre, un token y un puesto fijo. Una cuenta sirve para una sola sesión a la vez, más otra en la tarea en la que participa; cualquier otra sesión se rechaza y sus aprobaciones se quedan en la terminal.
- Aquí no se gestiona el inicio de sesión en Claude: todas las sesiones pueden compartir la misma cuenta de Claude.
- Los datos se guardan en `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) o `%AppData%\symphony` (Windows), o donde indiquen `-data` / `SYMPHONY_DATA`. `accounts.json` contiene los tokens y solo su propietario puede leerlo (0600); en Windows lo protegen los permisos de la carpeta del usuario.

## Seguridad

- El bridge solo acepta peticiones dirigidas a esta máquina y rechaza las peticiones de escritura que otro sitio web envíe a través de tu navegador: la página puede abrir sesiones, y eso equivale a ejecutar comandos en esta máquina.
- La página todavía no tiene inicio de sesión, así que **no** pongas el bridge a escuchar en una dirección que no sea de esta máquina.
- Los programas no están firmados. macOS puede rechazar una copia descargada con el navegador porque no puede verificar al desarrollador: haz clic en «Abrir igualmente» en Ajustes del Sistema → Privacidad y seguridad, o ejecuta una vez `xattr -d com.apple.quarantine symphony`. SmartScreen de Windows también puede avisarte: haz clic en «Más información» y luego en «Ejecutar de todas formas». El `checksums.txt` de la versión tiene el SHA-256 de cada archivo.

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

- Para trabajar en el mod: ejecuta `go run ./cmd/bridge -mod mods/symphony-bridge` y las sesiones abiertas desde la página recibirán tu copia del mod con `--plugin-dir`; en las sesiones que abras a mano, añade `--plugin-dir mods/symphony-bridge` por tu cuenta.
- Para trabajar en las páginas: ejecuta `go run ./cmd/bridge -web web` y verás tus cambios al recargar.
- Para ver la oficina sin sesiones reales: inicia un segundo bridge y llénalo de actividad inventada con `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo` y luego `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Traducciones: cada idioma es un archivo, `web/locales/<lang>.json` para las páginas y `internal/i18n/locales/<lang>.json` para el bridge, con las mismas claves que `en.json`; los pocos mensajes del mod están en `mods/symphony-bridge/hooks/messages.ts`. Las pruebas unitarias comprueban que cada idioma tenga todas las claves y los mismos `{placeholders}`, y la prueba de extremo a extremo `TestI18n` abre las páginas en todos los idiomas.
- Las pruebas de extremo a extremo necesitan claude, tmux (para las sesiones que las propias pruebas manejan) y Chromium o Chrome, así que se ejecutan en Linux y macOS. Por defecto usan el mod de este repositorio; define `SYMPHONY_E2E_MOD=installed` para usar el mod instalado. Las capturas de pantalla se guardan en `e2e/artifacts/`.
- Versiones: sube la versión en `mods/symphony-bridge/.claude-plugin/plugin.json`, añade una sección `## vX.Y.Z (date)` a `CHANGELOG.md`, lleva dev a main, luego etiqueta ese commit en main y sube la etiqueta (`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`); GitHub Actions comprueba que la etiqueta sea la versión de `plugin.json` y que la sección exista, compila el programa para todas las plataformas y publica una versión cuyas notas empiezan con esa sección (`.github/workflows/release.yml`; un sufijo como `-rc.1` la convierte en preversión). `.github/release.sh v0.7.0 dist` genera los mismos archivos en local.

| Carpeta | Qué es |
| --- | --- |
| `mods/symphony-bridge/` | El mod de Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | El marketplace de plugins que lee `claude plugin install` |
| `cmd/bridge/`, `internal/bridge/` | El bridge |
| `internal/i18n/` | La detección del idioma y las traducciones del bridge |
| `web/` | Las páginas de la oficina y de aprobaciones, con sus traducciones, integradas en el bridge |
| `cmd/demo/` | Actividad inventada para la oficina |
| `e2e/` | Pruebas de extremo a extremo |

## Licencia

[MIT](LICENSE)
