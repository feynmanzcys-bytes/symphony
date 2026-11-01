# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | **Deutsch** | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Setz ein Team aus Claude-Code-Sitzungen in ein Pixel-Art-Büro: Du siehst auf einen Blick, wer gerade arbeitet und wer auf dich wartet, sprichst mit jedem in seinem eigenen Fenster und erledigst dort Berechtigungsanfragen und Fragen per AskUserQuestion mit einem Klick.

![Das Büro](docs/office.png)

Die Seite gibt es in Designs: das Pixelbüro oben, die **HQ-Konsole**, eine Verwaltungskonsole mit Kennzahlenleiste, Belegschaft, Freigabewarteschlange, Projekt-Board und Aktivitätszeitleiste, den **Messenger**, die Sitzungen als Kollegen in einem Messenger, ihre Gesichter zeigen, was sie tun, das Gespräch in der Mitte, Profil und Rechner rechts, und **Mission Control**, die Wand eines Kontrollraums für einen zweiten Bildschirm: pro Sitzung eine Telemetriekachel, die Anfragen als Alarme, die Aktivität als Ticker. Wähle eines in der Leiste oben; die Bridge merkt es sich.

![Die HQ-Konsole](docs/hq.png)

![Der Messenger](docs/messenger.png)

![Mission Control](docs/mission.png)

## Installation

Am schnellsten geht es so: Öffne Claude Code und füge ein

> Installiere mir https://github.com/feynmanzcys-bytes/symphony

Claude Code liest diese README und erledigt alles aus dem nächsten Abschnitt. Wer von Hand installieren will, folgt denselben Schritten.

### Für KI-Agenten: die Installationsanleitung

Du liest das, weil ein Benutzer dich gebeten hat, symphony zu installieren. Führe die Schritte unten der Reihe nach in einer Shell aus und schließ mit dem Bericht aus Schritt 6 ab. Frag den Benutzer nichts, außer ein Schritt sagt es; triff die Routineentscheidungen selbst und liste sie im Bericht auf.

symphony besteht aus zwei Teilen und braucht beide:

1. **Das Programm `symphony`** (die Bridge): ein einzelnes Binary, ein lokaler Webserver unter `http://127.0.0.1:7788/`, der die Büroseite ausliefert und die Sitzungen ausführt. Es muss im `PATH` des Benutzers liegen, unter dem Namen `symphony`, weil der Mod es mit `symphony -background` startet, wenn eine Sitzung keine laufende Bridge findet.
2. **Das Plugin `symphony-bridge`** (der Mod): ein Claude-Code-Plugin aus dem Marktplatz dieses Repositorys, auf Benutzerebene installiert. Es wird nur in Sitzungen aktiv, die mit `SYMPHONY_TOKEN` gestartet wurden, und tut in allen anderen nichts.

#### Schritt 1: den Rechner prüfen

```bash
uname -sm                 # Linux/macOS: bestimmt das Archiv unten (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # braucht Claude Code 2.1.x (getestet mit 2.1.316)
git --version             # nur für gemeinsame Aufgaben; fehlt es, im Bericht vermerken, nicht abbrechen
symphony -version         # schon installiert? gibt "symphony v0.12.0 (darwin/arm64)" aus
curl -s http://127.0.0.1:7788/api/state | head -c 40   # läuft schon eine Bridge? gibt {"type":"snapshot",... aus
claude plugin list --json # Mod schon installiert? nach "id":"symphony-bridge@symphony" suchen
```

Unter Windows nimm PowerShell: `$env:PROCESSOR_ARCHITECTURE` ist `AMD64` oder `ARM64`, und statt curl `Invoke-WebRequest http://127.0.0.1:7788/api/state`.

Fehlt `claude`, hör auf und sag dem Benutzer, er soll zuerst Claude Code installieren. Sind Programm und Mod beide schon da, geh stattdessen zu „Aktualisieren“ unten.

#### Schritt 2: das Programm holen

Nimm die erste Möglichkeit, die funktioniert:

- **A. Mit `gh`** (funktioniert, solange dieses Repository privat ist, sofern `gh auth status` angemeldet ist):
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. Mit `curl`** (erst, wenn das Repository öffentlich ist; ein 404 heißt: A oder C nehmen):
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. Aus dem Quellcode** (Go 1.27 oder neuer; der Klon wird in Schritt 4 wiederverwendet):
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  Bau das Binary; `go run` reicht nicht, weil `-autostart` einen Build per `go run` ablehnt.

| System | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple Silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Jedes Archiv enthält das Programm (`symphony`, unter Windows `symphony.exe`), `LICENSE` und `README.md`. Die `checksums.txt` im Release enthält die SHA-256-Prüfsumme jedes Archivs (`sha256sum -c --ignore-missing checksums.txt`, unter macOS `shasum -a 256 -c`).

#### Schritt 3: in den PATH legen

Wähl einen Ordner, der schon im `PATH` des Benutzers liegt und ohne `sudo` beschreibbar ist: `~/.local/bin`, `/usr/local/bin` oder `/opt/homebrew/bin`, in dieser Reihenfolge. Gibt es keinen davon, leg `~/.local/bin` an, trag ihn in die rc-Datei der Shell ein (`~/.zshrc` oder `~/.bashrc`) und erwähn das im Bericht.

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: das Binary ist nicht signiert
symphony -version          # gibt "symphony v0.12.0 (linux/amd64)" aus: Version und Plattform
```

Unter Windows: `Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`, dann diesen Ordner zum `Path` des Benutzers hinzufügen (`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`) und eine neue Shell öffnen. SmartScreen kann beim ersten Start warnen: „Weitere Informationen“, dann „Trotzdem ausführen“.

Findet deine eigene Shell `symphony` danach nicht (ihr `PATH` wurde gelesen, bevor du ihn geändert hast), nimm für die restlichen Schritte den absoluten Pfad.

#### Schritt 4: den Mod installieren

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Das fügt das Repository als Plugin-Marktplatz `symphony` hinzu und installiert den Mod auf Benutzerebene, ohne Rückfragen. Dabei wird das Repository mit git geklont; solange es privat ist, braucht das also git-Zugriff auf GitHub. Scheitert es daran, nimm stattdessen einen Klon als Marktplatz (den aus Schritt 2 C, oder leg einen an): `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. Mit einem Klon als Marktplatz lesen die Sitzungen den Mod aus diesem Ordner, und ein `git pull` dort aktualisiert ihn.

Prüfen:

```bash
claude plugin list --json   # ein Eintrag hat "id":"symphony-bridge@symphony", "scope":"user", "enabled":true
```

- `"scope":"project"` oder `"local"`: Der Mod würde nur in einem Ordner geladen. Führ `claude plugin uninstall symphony-bridge@symphony` aus und installiere dann erneut mit `--scope user`.
- `"enabled":false`: `claude plugin enable symphony-bridge@symphony`.

#### Schritt 5: die Bridge starten

```bash
symphony -autostart on -open=false
```

Das startet die Bridge jetzt und bei jeder Anmeldung (ein systemd-Benutzerdienst unter Linux, ein launchd-Agent unter macOS, der Run-Schlüssel der Registry unter Windows) und gibt aus:

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

Hat der Benutzer gesagt, dass er sie nicht bei der Anmeldung starten will, führ stattdessen `symphony -background -open=false` aus: Das startet die Bridge losgelöst vom Terminal und gibt `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log` aus. Läuft schon eine, antworten beide Befehle mit `a symphony bridge is already running at http://127.0.0.1:7788/`; das ist in Ordnung, starte keine zweite.

Prüf die ganze Kette mit einer Anfrage:

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

Die Antwort enthält `"enabled":true` und `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.12.0"}`: Die Bridge läuft, sie hat `claude` gefunden, und der Mod ist dort installiert, wo die Sitzungen ihn laden. Bei `"enabled":false` sagt `mod.code`, was nicht stimmt, und `mod.fix` den Befehl, der es behebt: `mod_not_installed` oder `mod_project_scope` (zurück zu Schritt 4), `mod_disabled` (aktivieren), `mod_check_failed` (`claude plugin list --json` ist fehlgeschlagen, Claude Code ist also wohl zu alt). `"mod":null` heißt, dass die Bridge `claude` in ihrem `PATH` gar nicht gefunden hat (ihr Log sagt das): Leg es dorthin oder starte die Bridge mit `-claude /path/to/claude`.

Das Log der Bridge ist `bridge.log` im Datenordner (`~/.config/symphony` unter Linux, `~/Library/Application Support/symphony` unter macOS, `%AppData%\symphony` unter Windows); bei der Anmeldung gestartet, loggt sie unter Linux nach `journalctl --user -u symphony` und unter macOS nach `~/Library/Logs/symphony.log`.

#### Schritt 6: dem Benutzer berichten

Öffne das Büro im Browser (`open http://127.0.0.1:7788/` unter macOS, `xdg-open` unter Linux, `start` unter Windows) und sag dem Benutzer:

- Das Büro liegt unter `http://127.0.0.1:7788/`. Ein Klick auf das „+“ an einem leeren Platz stellt den ersten Mitarbeiter ein: Ordner wählen, und dort startet eine Claude-Code-Sitzung, innerhalb der Bridge; ihr Fenster auf der Seite ist ihr Chat.
- Im Büro erscheinen nur Sitzungen, die über die Seite gestartet wurden (oder von Hand mit `SYMPHONY_TOKEN`, siehe „Sitzungen starten“ unten). Seine anderen Claude-Code-Sitzungen, auch die, die gerade installiert, werden nicht angezeigt.
- Was wo installiert wurde: der Pfad des Programms, der Mod auf Benutzerebene, die Bridge bei der Anmeldung oder im Hintergrund gestartet, jede Änderung am `PATH`, und dass `git` fehlt, falls es fehlt.
- Wie man alles rückgängig macht: „Deinstallieren“ unten.

Regeln: Binde die Bridge an nichts außer diesen Rechner (`-addr` bleibt `127.0.0.1:7788`; die Seite hat keine Anmeldung), führ sie nicht mit `sudo` aus und starte keine zweite Bridge unter derselben Adresse.

#### Aktualisieren

Lad das neue Release wie in Schritt 2 herunter und ersetz das Programm wie in Schritt 3; der Mod zieht mit `claude plugin update symphony-bridge@symphony` nach (mit einem Klon als Marktplatz stattdessen `git pull` dort). Die laufende Bridge behält das alte Programm, bis sie neu startet: `symphony -autostart off`, dann `symphony -autostart on -open=false` (oder stopp und starte sie so, wie sie gestartet wurde). Wird die Bridge gestoppt, enden die Sitzungen, die sie gestartet hat; sag es also vorher dem Benutzer. Programm und Mod haben dieselbe Versionsnummer; [CHANGELOG.md](CHANGELOG.md) listet auf, was sich in jedem Release geändert hat.

#### Deinstallieren

```bash
symphony -autostart off                           # stoppt die Bridge und ihre Sitzungen und startet sie nicht mehr bei der Anmeldung
claude plugin uninstall symphony-bridge@symphony  # der Mod
claude plugin marketplace remove symphony         # der Marktplatz-Eintrag
rm ~/.local/bin/symphony                          # das Programm, wo auch immer Schritt 3 es abgelegt hat
```

Der Datenordner (die Konten mit ihren Tokens, der Nachrichtenverlauf, die Aufgaben) bleibt; lösch ihn, um alles zu entfernen.

## Was es kann

- **Jede Sitzung als Chat**: Klick auf einen Mitarbeiter, und sein Fenster öffnet sich in der Mitte der Seite: oben, wer er ist, darunter die Sitzung als Chat: die Eingaben, das Nachdenken des Modells (eingeklappt, „Denkt nach…“ in Orange, solange es dauert), seine Antworten, während sie eintreffen, die Werkzeugaufrufe und Nachrichten anderer Sitzungen in Lila. Schreib unten, und es erreicht die Sitzung als deine Eingabe; der Stopp-Knopf unterbricht sie.
- **Freigaben im Browser**: Berechtigungsanfragen (Befehle ausführen, Dateien bearbeiten) und Fragen per AskUserQuestion steigen als Karten über das Eingabefeld im Fenster dieses Mitarbeiters, und der rote Zähler in der Leiste oben bringt dich hin. Läuft die Bridge nicht oder kennt sie das Konto nicht, fällt die Sitzung wie gewohnt aufs Terminal zurück und bleibt nie hängen.
- **Ein Pixel-Art-Büro**: Jede Sitzung ist ein Mitarbeiter mit festem Platz. Grün ist frei, Gelb bei der Arbeit, Rot wartet auf deine Freigabe oder Antwort, Grau hat Feierabend. Wer nichts zu tun hat, bleibt erst am Platz und hängt am Handy oder zockt, dann zieht er los: Fernsehen auf dem Lounge-Sofa, Kicker, Fangen, ein Plausch am runden Tisch, Kaffee, ein Blick aus dem Fenster; nach einer Weile folgt ein Nickerchen, oft versteckt im Meetingraum. Die Größe der Figur richtet sich nach dem Modell, ihre Muskeln nach dem Denkaufwand. Das Büro ist ein Stockwerk im Gebäude einer großen Firma und füllt das Fenster: das Großraumbüro in der Mitte, links kleine Besprechungsräume für 3 bis 5 Personen mit Türschild, rechts der Meetingraum und die Lounge, und Aufzüge zum Kommen und Gehen. In der Leiste oben stellst du auch die Pixelgröße ein, in vier Stufen: dasselbe Büro, mit jeder Stufe in feineren Pixeln und mit mehr Details gezeichnet.
- **Jeden finden und zurückrufen**: Klick auf einen freien Mitarbeiter, und er geht an seinen Platz zurück (und bleibt dort bis zu seiner nächsten Aufgabe); das Suchfeld in der Leiste oben findet Mitarbeiter nach Name, Sitzungsname oder Ordner (mit `/` springst du hinein), und ein Klick auf einen Zähler daneben (frei, arbeitet, offen, Feierabend) hebt alle in diesem Zustand hervor. Mit dem Griff unter der Leiste schiebst du sie nach oben weg und wieder herunter.
- **Nachrichten zwischen Sitzungen**: Wenn sich Sitzungen gegenseitig Nachrichten schicken (SendMessage), fliegt ein Briefumschlag durchs Büro, und der Nachrichtenverlauf hält sie fest.
- **Sitzungen über die Seite starten, ohne dass sich etwas öffnet**: Klick auf das „+“ an einem leeren Platz, um einen neuen Mitarbeiter einzustellen, und wähle Ordner, Modell, Denkaufwand und Berechtigungsmodus (ein neuer Mitarbeiter startet mit allem erlaubt, Opus und dem meisten Nachdenken); klick bei einem Mitarbeiter, der Feierabend hat, auf „An die Arbeit“, und er kehrt an denselben Platz zurück. Die Sitzung läuft auf einem eigenen Terminal innerhalb der Bridge: kein Fenster öffnet sich, nichts nimmt dir den Fokus; „Terminal öffnen“ im Fenster des Mitarbeiters zeigt dieses Terminal, wenn du es willst, und es zu schließen ändert nichts.
- **Teams in den Besprechungsräumen**: Klick auf einen freien kleinen Besprechungsraum, schreib eine Aufgabe auf und wähl 2 bis 5 Mitglieder: Mitarbeiter, die jeweils eine zweite Sitzung nur für die Aufgabe bekommen, oder Berater, die nur für dieses eine Mal dazukommen. Standardmäßig leitet das erste Mitglied und verteilt die Teile per SendMessage; jedes Mitglied arbeitet in seinem eigenen Git-Worktree auf seinem eigenen Branch, sodass niemand jemandem in die Dateien pfuscht. Bis die Aufgabe endet, sitzen die Mitglieder im Besprechungsraum.
- **10 Sprachen**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) und Русский. Die Seite richtet sich nach der Sprache deines Browsers und hat in der Leiste oben eine Sprachauswahl; die Meldungen der Bridge folgen `-lang`, sonst deiner Systemsprache (`LANG` und Ähnliches), sonst Englisch; die des Mods folgen `SYMPHONY_LANG` (bei einer über die Seite gestarteten Sitzung die Sprache der Seite), sonst deiner Systemsprache, sonst Englisch.
- **Nur lokal**: Die Bridge ist ein einzelnes Go-Programm (nur Standardbibliothek; die End-to-End-Tests nutzen zusätzlich chromedp), die Seiten sind reines JavaScript, und nichts hängt von einem Drittanbieterdienst ab.

![Freigaben im Browser](docs/approve.png)

## Voraussetzungen

- Claude Code 2.1.316 (mit dieser Version getestet). Der Mod nutzt die Function Hooks von Claude Code, die noch eine Early-Access-API sind; führe also nach einem Update von Claude Code die End-to-End-Tests aus.
- Die Bridge gibt es als fertiges Programm für Linux, macOS und Windows; Go 1.27 oder neuer brauchst du nur, um sie aus dem Quellcode zu bauen.
- `git`, nur für gemeinsame Aufgaben: Jedes Mitglied einer Aufgabe arbeitet in einem Git-Worktree.
- Sonst nichts: Über die Seite gestartete Sitzungen laufen auf jeder Plattform innerhalb der Bridge.

## Die Bridge betreiben

`symphony` startet die Bridge und öffnet das Büro in deinem Browser unter <http://127.0.0.1:7788/> (`-open=false` schaltet das ab); <http://127.0.0.1:7788/approve> ist eine einfachere Seite nur für Freigaben. Startest du die Bridge erneut, während sie schon läuft, öffnet sich nur das Büro. Unter Windows läuft `symphony.exe` per Doppelklick in einem eigenen Fenster, und wenn du das Fenster schließt, stoppt die Bridge.

| Flag | Standard | Wozu |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Adresse, auf der die Bridge lauscht. Die Seite hat noch keine Anmeldung, also binde die Bridge nur an diesen Rechner |
| `-data` | `$SYMPHONY_DATA`, sonst `symphony` in deinem Konfigurationsordner | Der Datenordner: die Konten mit ihren Tokens (`accounts.json`), der Nachrichtenverlauf (`messages.json`), die Aufgaben (`tasks.json`) und das Log von `-background` (`bridge.log`) |
| `-claude` | `claude` | Die ausführbare Datei von claude |
| `-lang` | deine Systemsprache, sonst `en` | Sprache der Bridge-Meldungen (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` oder `LANG` legen den Standard fest) |
| `-mod` | leer | Für die Entwicklung: gibt Sitzungen per `--plugin-dir` den Mod aus diesem Ordner mit, statt der installierten Kopie |
| `-web` | leer | Für die Entwicklung: liefert die Seiten aus diesem Ordner aus, sodass Änderungen nach dem Neuladen sichtbar sind |
| `-open` | `true` | Öffnet das Büro im Browser, sobald die Bridge läuft; läuft unter `-addr` schon eine Bridge, wird nur ihr Büro geöffnet |
| `-autostart` | — | `on`: Startet die Bridge bei jeder Anmeldung, mit den übrigen angegebenen Optionen, und startet sie gleich; `off`: macht das rückgängig |
| `-background` | — | Startet die Bridge im Hintergrund, losgelöst vom Terminal und mit Log in `bridge.log` im Datenordner, und kehrt zurück, sobald sie antwortet |
| `-version` | — | Gibt die Version aus und beendet sich |

**Beim Anmelden starten**: `symphony -autostart on` lässt die Bridge bei jeder Anmeldung starten, mit den gleich mitgegebenen Optionen (etwa `-addr`), und startet sie sofort; `symphony -autostart off` macht das rückgängig. Unter Linux ist das ein systemd-Benutzerdienst (`journalctl --user -u symphony` zeigt sein Log), unter macOS ein launchd-Agent (Log in `~/Library/Logs/symphony.log`), unter Windows ein Wert unter dem Run-Schlüssel der Registry (die Bridge läuft in einem Konsolenfenster; schließt du es, ist sie bis zur nächsten Anmeldung aus). Dafür braucht es das Programm symphony selbst, nicht `go run`.

**Aus dem Quellcode**, mit Go 1.27 oder neuer: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Sitzungen starten

**Über die Seite**: Klick auf das „+“ an einem leeren Platz, gib einen Namen ein und wähle Ordner und Optionen. Die Bridge führt `claude` auf einem eigenen Terminal innerhalb der Bridge aus: Es öffnet sich kein Fenster. Das Fenster des Mitarbeiters zeigt die Sitzung als Chat, und sein Knopf „Terminal öffnen“ öffnet ein Terminalfenster, das mit der Sitzung verbunden ist (`symphony attach <id>` tut dasselbe aus jedem Terminal; `Ctrl-]` oder das Schließen des Fensters trennt die Verbindung, und die Sitzung läuft weiter). Die Sitzungen sind Kindprozesse der Bridge: Wird die Bridge beendet, enden sie.

**Aus einem Terminal**: Leg ein Konto an, um ein Token zu bekommen, und starte `claude` damit:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # die Antwort enthält das Token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Empfohlen ist der Berechtigungsmodus `manual`: Im Modus `auto` wird bei den meisten Aktionen niemand gefragt, also kommen kaum Freigaben auf der Seite an. Läuft die Bridge nicht unter der Standardadresse, setz zusätzlich `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` legt die Sprache der Mod-Meldungen fest (sonst deine Systemsprache, sonst Englisch).

Antwortet unter der Adresse der Bridge auf diesem Rechner nichts, startet die Sitzung die Bridge selbst mit `symphony -background -open=false -addr <that address>` (wenn `symphony` in deinem PATH liegt), und die Bridge läuft nach dem Ende der Sitzung weiter. Setz `SYMPHONY_DATA`, wenn deine Konten nicht im Standard-Datenordner liegen.

## Gemeinsame Aufgaben

In einem kleinen Besprechungsraum arbeitet ein Team aus 2 bis 5 Sitzungen gemeinsam an einer Aufgabe. Aufgaben brauchen `git` und den Mod in Version 0.7.0 (`claude plugin update symphony-bridge@symphony`).

**Eine Aufgabe starten**: Klick auf einen freien kleinen Besprechungsraum und beschreib, was zu tun ist. Hak die Mitglieder in der gewünschten Reihenfolge an (2 bis zu den Plätzen im Raum), wähl für jedes Mitglied Modell und Denkaufwand und für das ganze Team den Berechtigungsmodus (standardmäßig alles erlaubt), dann klick auf „Aufgabe starten“. Es gibt zwei Arten von Mitgliedern:

- **Mitarbeiter**: Jeder bekommt eine zweite Sitzung nur für die Aufgabe, die im Ordner des Mitarbeiters arbeitet; seine eigene Sitzung am Platz bleibt, wie sie ist. Ein Mitarbeiter ist immer nur in einer Aufgabe, und wer noch nie gearbeitet hat, hat noch keinen Ordner: Klick vorher einmal auf „An die Arbeit“.
- **Berater**: Teilnehmer nur für diese eine Aufgabe, angelegt mit den üblichen Feldern (Name, Ordner, Modell, Denkaufwand). Sie haben keinen Platz, kommen mit dem Aufzug und sind wieder weg, sobald die Aufgabe endet.

**Die Leitung**: Standardmäßig leitet das erste Mitglied: Es teilt die Aufgabe auf, verteilt die Teile per SendMessage, prüft die Arbeit, führt die Branches der anderen in seinen eigenen zusammen und sagt dir, was erledigt wurde. Nimm den Haken bei der Leitung weg, dann bekommt jeder die ganze Aufgabe, und die Mitglieder teilen sie unter sich auf. So oder so gibt die Bridge jedem Mitglied seine erste Eingabe: wer im Team ist (die Mitglieder erreichen einander per SendMessage über ihren Namen), den Branch jedes Einzelnen und die Regeln.

**Git**: Jedes Mitglied arbeitet in seinem eigenen Git-Worktree auf seinem eigenen Branch `symphony/<task id>/<member>` (etwa `symphony/T1/bob`; ein zweites Mitglied mit demselben Namen bekommt `-2`) seines Ordners, sodass niemand jemandem in die Dateien pfuscht und dein eigener Checkout unverändert bleibt. Die Worktrees liegen im Ordner unter `.symphony/worktrees/`, vor Git versteckt durch `.git/info/exclude` (deine `.gitignore` bleibt unberührt). Wenn die Aufgabe endet, wird alles, was ein Mitglied noch nicht committet hat, auf seinen Branch committet, damit nichts verloren geht, und die Worktrees werden entfernt.

- In deinen eigenen Repositorys wird nichts zusammengeführt: Die Branches bleiben, damit du sie zusammenführst; gibt es eine Leitung, sammelt deren Branch die Arbeit aller.
- Ein Ordner, der kein Git-Repository ist, wird für die Aufgabe zu einem (`git init` und ein Commit mit allem, was darin liegt) und bleibt es. Wenn die Aufgabe endet, werden die Branches der Mitglieder darin zusammengeführt, sodass der Ordner das Ergebnis enthält; das `.git` und die Branches bleiben erhalten.
- Schlägt ein Merge fehl, wird er rückgängig gemacht und die Basis bleibt, wie sie war; die Branches bleiben mit der Arbeit darauf erhalten, und das Aufgabenfenster zeigt, welcher Branch einen Konflikt hat.

**Im Büro**: Die Mitglieder sitzen die ganze Aufgabe über am Tisch des Raums, und am Platz eines Mitarbeiters steht so lange ein leerer Stuhl. Das Türschild des Raums zeigt die ID der Aufgabe; sein Licht, grün, solange der Raum frei ist, wird gelb, während die Aufgabe läuft, und rot, während ein Mitglied auf deine Freigabe oder Antwort wartet. Ein Klick auf den Raum öffnet das Aufgabenfenster: die Aufgabe, die Mitglieder mit ihren Ordnern und Branches, den Team-Chat und „Aufgabe beenden“. Auch ein Klick auf die Figur eines Mitglieds im Raum öffnet das Aufgabenfenster; ein Klick auf ein Mitglied darin öffnet sein Chatfenster direkt daneben, genau wie das Fenster eines Mitarbeiters, immer nur für ein Mitglied (ein Klick auf ein anderes Mitglied wechselt). Der rote Zähler in der Leiste oben und das Suchfeld öffnen ein Mitglied einer Aufgabe genauso.

**Der Team-Chat**: Das Aufgabenfenster listet die Nachrichten, die sich die Mitglieder gegenseitig schicken, jede mit der Pixelfigur und dem Namen des Absenders, und der Text beginnt mit `@` und dem Empfänger. Darunter ist ein Feld, in dem du selbst ans Team schreibst: `@bob-T1 …` geht nur an dieses Mitglied (mehrere `@names` gehen an alle davon; tippst du `@`, öffnet sich eine Liste der Mitglieder, aus der du mit den Pfeiltasten, Enter oder einem Klick wählst), und eine Nachricht ohne `@` geht an alle in der Aufgabe. Enter sendet, Umschalt+Enter beginnt eine neue Zeile.

Eine Aufgabe endet, wenn du sie im Aufgabenfenster beendest oder wenn ihre letzte Sitzung endet: Die Bridge stoppt die Sitzungen der Mitglieder, kümmert sich wie oben beschrieben um die Git-Seite und löscht die Berater. Eine Aufgabe, die noch läuft, wenn die Bridge stoppt, wird beim nächsten Start der Bridge auf dieselbe Weise beendet.

## Konten und Plätze

- Eine Identität in symphony besteht aus einem Namen, einem Token und einem festen Platz. Ein Konto kann immer nur von einer Sitzung zugleich genutzt werden, dazu von einer in der Aufgabe, an der es teilnimmt; jede weitere Sitzung wird abgewiesen, und ihre Freigaben bleiben im Terminal.
- Claude-Logins werden hier nicht verwaltet: Alle Sitzungen können sich ein Claude-Login teilen.
- Die Daten liegen in `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) oder `%AppData%\symphony` (Windows), oder dort, wo `-data` / `SYMPHONY_DATA` es angibt. `accounts.json` enthält die Tokens und ist nur für den Besitzer lesbar (0600); unter Windows übernehmen das die Zugriffsrechte des Benutzerordners.

## Sicherheit

- Die Bridge nimmt nur Anfragen an, die an diesen Rechner gerichtet sind, und lehnt schreibende Anfragen ab, die eine andere Website über deinen Browser schickt: Die Seite kann Sitzungen starten, also Befehle auf diesem Rechner ausführen.
- Die Seite hat noch keine Anmeldung, also binde die Bridge **nicht** an eine andere Adresse als die dieses Rechners.
- Die Programme sind nicht signiert. macOS lehnt eine mit dem Browser heruntergeladene Kopie womöglich ab, weil es den Entwickler nicht überprüfen kann: Klick unter Systemeinstellungen → Datenschutz & Sicherheit auf „Dennoch öffnen“ oder führe einmal `xattr -d com.apple.quarantine symphony` aus. Unter Windows kann SmartScreen ebenfalls warnen: Klick auf „Weitere Informationen“ und dann auf „Trotzdem ausführen“. Die `checksums.txt` im Release enthält die SHA-256-Prüfsumme jedes Archivs.

## Plattformen

| System | Über die Seite gestartete Sitzungen | „Terminal öffnen“ | Getestet |
| --- | --- | --- | --- |
| Linux | Ein Pseudoterminal innerhalb der Bridge | Ein grafisches Terminal (x-terminal-emulator, gnome-terminal, konsole, xterm) mit `symphony attach` | Vollständige End-to-End-Tests |
| macOS | Ein Pseudoterminal innerhalb der Bridge | Terminal.app mit `symphony attach` | Vollständige End-to-End-Tests |
| Windows | Eine Pseudokonsole (ConPTY) innerhalb der Bridge | Ein neues Konsolenfenster mit `symphony attach` | Nur Cross-Kompilierung und Unit-Tests |

Die Bridge bestätigt „trust this folder“ auf jeder Plattform für dich.

## Entwicklung

```bash
go test -race ./...                         # Unit-Tests für Bridge und Übersetzungen
claude plugin test mods/symphony-bridge     # Unit-Tests für den Mod
claude plugin validate .                    # Marktplatz prüfen
claude plugin validate mods/symphony-bridge  # Mod prüfen
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # End-to-End: startet echte Sitzungen und verbraucht Tokens
```

- Am Mod arbeiten: `go run ./cmd/bridge -mod mods/symphony-bridge`, dann bekommen über die Seite gestartete Sitzungen deine Kopie des Mods per `--plugin-dir`; Sitzungen, die du von Hand startest, gibst du `--plugin-dir mods/symphony-bridge` selbst mit.
- An den Seiten arbeiten: `go run ./cmd/bridge -web web`, dann zeigt ein Neuladen deine Änderungen.
- Das Büro ohne echte Sitzungen ansehen: Starte eine zweite Bridge und füll sie mit ausgedachter Aktivität: `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`, dann `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Übersetzungen: Jede Sprache ist eine Datei, `web/locales/<lang>.json` für die Seiten und `internal/i18n/locales/<lang>.json` für die Bridge, mit denselben Schlüsseln wie `en.json`; die paar Meldungen des Mods stehen in `mods/symphony-bridge/hooks/messages.ts`. Die Unit-Tests prüfen, dass jede Sprache alle Schlüssel und dieselben `{placeholders}` hat, und der End-to-End-Test `TestI18n` öffnet die Seiten in jeder Sprache.
- Die End-to-End-Tests brauchen claude, tmux (für die Sitzungen, die die Tests selbst steuern) und Chromium oder Chrome, laufen also unter Linux und macOS. Standardmäßig nutzen sie den Mod aus diesem Repository; mit `SYMPHONY_E2E_MOD=installed` nehmen sie den installierten Mod. Screenshots landen in `e2e/artifacts/`.
- Releases: Erhöh die Version in `mods/symphony-bridge/.claude-plugin/plugin.json`, füg in `CHANGELOG.md` einen Abschnitt `## vX.Y.Z (date)` hinzu, bring dev nach main, versieh dann diesen Commit auf main mit einem Tag und pushe den Tag (`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`); GitHub Actions prüft, dass der Tag die Version aus `plugin.json` ist und dass der Abschnitt existiert, baut das Programm für alle Plattformen und veröffentlicht ein Release, dessen Release Notes mit diesem Abschnitt beginnen (`.github/workflows/release.yml`; ein Suffix wie `-rc.1` macht daraus ein Pre-Release). `.github/release.sh v0.7.0 dist` baut dieselben Archive lokal.

| Ordner | Was drin ist |
| --- | --- |
| `mods/symphony-bridge/` | Der Claude-Code-Mod (TypeScript) |
| `.claude-plugin/marketplace.json` | Der Plugin-Marktplatz, den `claude plugin install` liest |
| `cmd/bridge/`, `internal/bridge/` | Die Bridge |
| `internal/i18n/` | Spracherkennung und die Übersetzungen der Bridge |
| `web/` | Büro- und Freigabeseite samt Übersetzungen, in die Bridge eingebaut |
| `cmd/demo/` | Ausgedachte Aktivität fürs Büro |
| `e2e/` | End-to-End-Tests |

## Lizenz

[MIT](LICENSE)
