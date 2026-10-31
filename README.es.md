# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | **Español** | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Pon a trabajar a un equipo de sesiones de Claude Code en una oficina en pixel art: ve de un vistazo quién está trabajando y quién te está esperando, y atiende las solicitudes de permiso y las preguntas de AskUserQuestion con un solo clic en una ventana emergente de la página web.

![La oficina](docs/office.png)

## Qué hace

- **Aprobaciones en el navegador**: las solicitudes de permiso (para ejecutar comandos o editar archivos) y las preguntas de AskUserQuestion llegan a la página web. Si el bridge no está en marcha o no reconoce la cuenta, la sesión vuelve a la terminal como siempre y nunca se queda colgada.
- **Una oficina en pixel art**: cada sesión es un empleado con su propio puesto fijo. El verde indica que está libre; el amarillo, que está trabajando; el rojo, que espera tu aprobación o tu respuesta, y el gris, que está fuera de servicio. Si no tienen nada que hacer, primero se quedan en su puesto con el teléfono o los videojuegos y luego se van de paseo: ven la tele en el sofá de la zona de descanso, juegan al futbolín o a pillar, charlan en la mesa redonda, se sirven un café o miran por la ventana; al rato se echan una siesta, casi siempre escondidos en la sala de reuniones. El tamaño del personaje depende del modelo, y sus músculos, del esfuerzo de razonamiento. La oficina es una planta del edificio de una gran empresa y llena la ventana: la zona de trabajo en el centro, pequeñas salas de reuniones para 3 a 5 personas con su placa a la izquierda, la sala de reuniones y la zona de descanso a la derecha, y ascensores por los que se entra y se sale. En la barra superior también eliges el tamaño del píxel, en cuatro pasos: la misma oficina, dibujada con píxeles más finos y más detalle en cada paso.
- **Encuentra a cualquiera y llámalo**: haz clic en un empleado libre y volverá a su puesto (y se quedará allí hasta su siguiente tarea); el buscador de la barra superior encuentra empleados por nombre o carpeta (pulsa `/` para ir a él), y un clic en un contador junto a él (libre, trabajando, pendiente, fuera de servicio) resalta a todos los que están en ese estado. La pestaña bajo la barra la recoge hacia arriba y la vuelve a bajar.
- **Mensajes entre sesiones**: cuando las sesiones se envían mensajes (SendMessage), un sobre cruza la oficina volando y el registro de mensajes los guarda.
- **Abre sesiones desde la página**: haz clic en el "+" de un puesto vacío para contratar a alguien nuevo y elige la carpeta, el modelo, el esfuerzo de razonamiento y el modo de permisos; en la ficha de alguien que está fuera de servicio, haz clic en "A trabajar" para que vuelva a su mismo puesto.
- **10 idiomas**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) y Русский. La página usa el idioma de tu navegador y tiene un selector de idioma en la barra superior; los mensajes del bridge usan el idioma de tu sistema (o el que indiques con `-lang`), y los del mod, el de la página que abrió la sesión.
- **Todo en local**: el bridge es un único programa en Go (solo con la biblioteca estándar), las páginas son JavaScript puro y nada depende de servicios de terceros.

![Aprobaciones en el navegador](docs/approve.png)

## Requisitos

- Claude Code 2.1.314 (probado con esta versión). El mod usa los function hooks de Claude Code, que todavía son una API en acceso anticipado, así que ejecuta las pruebas de extremo a extremo después de actualizar Claude Code.
- El bridge viene como programa listo para usar en Linux, macOS y Windows; Go 1.27 o posterior solo hace falta para ejecutarlo desde el código fuente.
- Para abrir sesiones desde la página: en Linux y macOS, lo ideal es tmux.

## Instalación

### 1. Instala el mod

En Claude Code, escribe:

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Pulsa `y` para añadir el marketplace y luego elige un ámbito (el predeterminado es el de usuario). O instálalo desde la línea de comandos:

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Si ya tienes el repositorio clonado, también puedes usar tu clon como marketplace. Así el mod y el bridge son el mismo código, y después de un `git pull` cada sesión nueva usa la versión nueva:

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

Instalado en el ámbito de usuario, el mod se carga en todas las sesiones de Claude Code, pero solo actúa en las que se inician con `SYMPHONY_TOKEN`; en las demás no hace nada.

Para actualizar: `claude plugin update symphony-bridge@symphony`. Para desinstalar: `claude plugin uninstall symphony-bridge@symphony`.

### 2. Inicia el bridge

Descarga el archivo para tu sistema desde la [última versión](https://github.com/feynmanzcys-bytes/symphony/releases/latest), descomprímelo y ejecuta el programa `symphony` que contiene:

| Sistema | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Por ejemplo, en Linux:

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

En Windows, descomprime el zip y ejecuta `symphony.exe`; con doble clic se abre en su propia ventana, y al cerrarla se detiene el bridge.

Los programas no están firmados, así que la primera ejecución puede quedar bloqueada. macOS puede rechazar una copia descargada con el navegador porque no puede verificar al desarrollador: haz clic en "Abrir igualmente" en Ajustes del Sistema → Privacidad y seguridad, o ejecuta una vez `xattr -d com.apple.quarantine symphony`. SmartScreen de Windows también puede avisarte: haz clic en "Más información" y luego en "Ejecutar de todas formas". El `checksums.txt` de la versión tiene el SHA-256 de cada archivo.

El programa y el mod comparten número de versión: `symphony -version` muestra el del programa y `claude plugin list`, el del mod. Para actualizar, descarga la nueva versión y ejecuta `claude plugin update symphony-bridge@symphony`. [CHANGELOG.md](CHANGELOG.md) recoge qué cambió en cada versión.

O ejecuta el bridge desde el código fuente, con Go 1.27 o posterior:

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

La oficina se abre en tu navegador en <http://127.0.0.1:7788/> (`-open=false` lo desactiva); <http://127.0.0.1:7788/approve> es una página más sencilla, solo para aprobaciones. Si inicias el bridge otra vez mientras ya está en marcha, solo se abre la oficina.

| Opción | Predeterminado | Qué hace |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Dirección en la que escucha. La página todavía no tiene inicio de sesión, así que no uses una dirección que no sea de esta máquina |
| `-data` | `$SYMPHONY_DATA`; si no, `symphony` en tu carpeta de configuración | Dónde se guardan las cuentas (con sus tokens) y el registro de mensajes |
| `-terminal` | `auto` | Terminal para las sesiones abiertas desde la página: `auto`, `tmux` o `window` |
| `-claude` | `claude` | El ejecutable de claude |
| `-lang` | el idioma de tu sistema | Idioma de los mensajes del bridge |
| `-mod` | vacío | Para desarrollo: pasa a las sesiones el mod de esta carpeta con `--plugin-dir`, en lugar de la copia instalada |
| `-web` | vacío | Para desarrollo: sirve las páginas desde esta carpeta, así los cambios se ven al recargar |
| `-open` | `true` | Abre la oficina en el navegador cuando el bridge arranca; si ya hay un bridge en `-addr`, solo abre su oficina |
| `-autostart` | — | `on`: inicia el bridge cada vez que inicias sesión, con las demás opciones dadas, y lo inicia ya; `off`: lo deshace |
| `-background` | — | Inicia el bridge en segundo plano, separado de la terminal y con su registro en `bridge.log` de la carpeta de datos, y vuelve cuando responde |
| `-version` | — | Muestra la versión y sale |

**Iniciarlo al iniciar sesión**: `symphony -autostart on` hace que el bridge se inicie cada vez que inicias sesión, con las opciones que pases junto (por ejemplo `-addr`), y lo inicia ya; `symphony -autostart off` lo deshace. En Linux es un servicio de usuario de systemd (`journalctl --user -u symphony` muestra su registro), en macOS un agente de launchd (registro en `~/Library/Logs/symphony.log`) y en Windows un valor bajo la clave Run del registro (el bridge corre en una ventana de consola; al cerrarla se detiene hasta el próximo inicio de sesión). Necesita el programa symphony en sí, no `go run`.

### 3. Abre sesiones

**Desde la página**: haz clic en el "+" de un puesto vacío, escribe un nombre y elige la carpeta y las opciones. El bridge abre una terminal que ejecuta `claude`; con tmux, la sesión se llama `sym-<id>` y `tmux attach -t sym-<id>` te lleva a ella.

**Desde una terminal**: crea una cuenta para obtener un token y luego inicia `claude` con él:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # la respuesta incluye el token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Se recomienda el modo de permisos `manual`: en el modo `auto` la mayoría de las acciones no le preguntan a nadie, así que llegan pocas aprobaciones a la página. Si el bridge no está en la dirección predeterminada, define también `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` elige el idioma de los mensajes del mod.

Si nada responde en la dirección del bridge en esta máquina, la sesión lo inicia ella misma con `symphony -background` (si `symphony` está en tu PATH), y el bridge sigue en marcha cuando la sesión termina. Define `SYMPHONY_DATA` si tus cuentas no están en la carpeta de datos predeterminada.

## Cuentas y puestos

- En symphony, una identidad es un nombre, un token y un puesto fijo. Una cuenta sirve para una sola sesión a la vez; si se conecta una segunda sesión, se rechaza y sus aprobaciones se quedan en la terminal.
- Aquí no se gestiona el inicio de sesión en Claude: todas las sesiones pueden compartir la misma cuenta de Claude.
- Los datos se guardan en `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) o `%AppData%\symphony` (Windows). `accounts.json` contiene los tokens y solo su propietario puede leerlo (0600); en Windows lo protegen los permisos de la carpeta del usuario.

## Seguridad

- El bridge solo acepta peticiones dirigidas a esta máquina y rechaza las peticiones de escritura que otro sitio web envíe a través de tu navegador: la página puede abrir sesiones, y eso equivale a ejecutar comandos en esta máquina.
- La página todavía no tiene inicio de sesión, así que **no** pongas el bridge a escuchar en una dirección que no sea de esta máquina.

## Plataformas

| Sistema | Terminal para las sesiones abiertas desde la página | Pruebas |
| --- | --- | --- |
| Linux | tmux si está instalado; si no, una terminal gráfica | Pruebas de extremo a extremo completas |
| macOS | Terminal.app por defecto; `-terminal tmux` para usar tmux | Solo compilación cruzada y pruebas unitarias |
| Windows | Una ventana de consola nueva | Solo compilación cruzada y pruebas unitarias |

Solo tmux puede aceptar "confiar en esta carpeta" por ti. Con una ventana de terminal, la primera sesión en una carpeta nueva necesita que hagas un clic en esa ventana.

## Desarrollo

```bash
go test -race ./...                         # pruebas unitarias del bridge y de las traducciones
claude plugin test mods/symphony-bridge     # pruebas unitarias del mod
claude plugin validate .                    # comprueba el marketplace y el mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # de extremo a extremo: abre sesiones reales y gasta tokens
```

- Para trabajar en el mod: ejecuta `go run ./cmd/bridge -mod mods/symphony-bridge` y las sesiones abiertas desde la página recibirán tu copia del mod con `--plugin-dir`; en las sesiones que abras a mano, añade `--plugin-dir mods/symphony-bridge` por tu cuenta.
- Para trabajar en las páginas: ejecuta `go run ./cmd/bridge -web web` y verás tus cambios al recargar.
- Para ver la oficina sin sesiones reales: inicia un segundo bridge y llénalo de actividad inventada con `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo` y luego `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Traducciones: cada idioma es un archivo, `web/locales/<lang>.json` para las páginas y, para el bridge, `internal/i18n/locales/<lang>.json`, con las mismas claves que `en.json`; los pocos mensajes del mod están en `mods/symphony-bridge/hooks/messages.ts`. Las pruebas unitarias comprueban que cada idioma tenga todas las claves y los mismos `{placeholders}`, y la prueba de extremo a extremo `TestI18n` abre las páginas en todos los idiomas.
- Las pruebas de extremo a extremo necesitan claude, tmux y chromium, así que se ejecutan en Linux y en macOS con tmux. Por defecto usan el mod de este repositorio; define `SYMPHONY_E2E_MOD=installed` para usar el mod instalado. Las capturas de pantalla se guardan en `e2e/artifacts/`.
- Versiones: etiqueta un commit de main con su versión y sube la etiqueta (`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`); GitHub Actions compila entonces el programa para todas las plataformas y publica una versión (`.github/workflows/release.yml`). La etiqueta debe ser la versión del mod en `plugin.json`. `.github/release.sh v0.3.0 dist` genera los mismos archivos en local. Cada versión necesita una sección en `CHANGELOG.md`, que encabeza sus notas.

| Carpeta | Qué es |
| --- | --- |
| `mods/symphony-bridge/` | El mod de Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | El marketplace de plugins que lee `/plugin install` |
| `cmd/bridge/`, `internal/bridge/` | El bridge |
| `internal/i18n/` | La detección del idioma y las traducciones del bridge |
| `web/` | Las páginas de la oficina y de aprobaciones, con sus traducciones, integradas en el bridge |
| `cmd/demo/` | Actividad inventada para la oficina |
| `e2e/` | Pruebas de extremo a extremo |

## Licencia

[MIT](LICENSE)
