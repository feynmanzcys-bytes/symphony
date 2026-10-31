# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | **Deutsch** | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Setz ein Team aus Claude-Code-Sitzungen in ein Pixel-Art-Büro: Du siehst auf einen Blick, wer gerade arbeitet und wer auf dich wartet, und erledigst Berechtigungsanfragen und Fragen per AskUserQuestion mit einem Klick in einem Pop-up im Browser.

![Das Büro](docs/office.png)

## Was es kann

- **Freigaben im Browser**: Berechtigungsanfragen (Befehle ausführen, Dateien bearbeiten) und Fragen per AskUserQuestion landen auf der Webseite. Läuft die Bridge nicht oder kennt sie das Konto nicht, fällt die Sitzung wie gewohnt aufs Terminal zurück und bleibt nie hängen.
- **Ein Pixel-Art-Büro**: Jede Sitzung ist ein Mitarbeiter mit festem Platz. Grün ist frei, Gelb bei der Arbeit, Rot wartet auf deine Freigabe oder Antwort, Grau hat Feierabend. Wer nichts zu tun hat, bleibt erst am Platz und hängt am Handy oder zockt, dann zieht er los: Fernsehen auf dem Lounge-Sofa, Kicker, Fangen, ein Plausch am runden Tisch, Kaffee, ein Blick aus dem Fenster; nach einer Weile folgt ein Nickerchen, meist versteckt im Meetingraum. Die Größe der Figur richtet sich nach dem Modell, ihre Muskeln nach dem Denkaufwand. Das Büro ist ein Stockwerk im Gebäude einer großen Firma und füllt das Fenster: das Großraumbüro in der Mitte, links kleine Besprechungsräume für 3 bis 5 Personen mit Türschild, rechts der Meetingraum und die Lounge, und Aufzüge zum Kommen und Gehen. In der Leiste oben stellst du auch die Pixelgröße ein, in vier Stufen: dasselbe Büro, mit jeder Stufe in feineren Pixeln und mit mehr Details gezeichnet.
- **Jeden finden und zurückrufen**: Klick auf einen freien Mitarbeiter, und er geht an seinen Platz zurück (und bleibt dort bis zu seiner nächsten Aufgabe); das Suchfeld in der Leiste oben findet Mitarbeiter nach Name oder Ordner (mit `/` springst du hinein), und ein Klick auf einen Zähler daneben (frei, arbeitet, offen, Feierabend) hebt alle in diesem Zustand hervor. Mit dem Griff unter der Leiste schiebst du sie nach oben weg und wieder herunter.
- **Nachrichten zwischen Sitzungen**: Wenn sich Sitzungen gegenseitig Nachrichten schicken (SendMessage), fliegt ein Briefumschlag durchs Büro, und der Nachrichtenverlauf hält sie fest.
- **Sitzungen über die Seite starten**: Klick auf das „+“ an einem leeren Platz, um einen neuen Mitarbeiter einzustellen, und wähle Ordner, Modell, Denkaufwand und Berechtigungsmodus; klick bei einem Mitarbeiter, der Feierabend hat, auf „An die Arbeit“, und er kehrt an seinen alten Platz zurück.
- **10 Sprachen**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) und Русский. Die Seite richtet sich nach der Sprache deines Browsers und hat in der Leiste oben eine Sprachauswahl; die Meldungen der Bridge folgen deiner Systemsprache (oder `-lang`), die des Mods der Seite, die die Sitzung gestartet hat.
- **Nur lokal**: Die Bridge ist ein einzelnes Go-Programm (nur Standardbibliothek), die Seiten sind reines JavaScript, und nichts hängt von einem Drittanbieterdienst ab.

![Freigaben im Browser](docs/approve.png)

## Voraussetzungen

- Claude Code 2.1.314 (mit dieser Version getestet). Der Mod nutzt die Function Hooks von Claude Code, die noch eine Early-Access-API sind; führe also nach einem Update von Claude Code die End-to-End-Tests aus.
- Die Bridge gibt es als fertiges Programm für Linux, macOS und Windows; Go 1.27 oder neuer brauchst du nur, um sie aus dem Quellcode zu starten.
- Zum Starten von Sitzungen über die Seite: Unter Linux und macOS ist tmux am besten.

## Installation

### 1. Den Mod installieren

Gib in Claude Code ein:

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Drück `y`, um den Marktplatz hinzuzufügen, und wähle dann einen Bereich (Standard ist die Benutzerebene, „user scope“). Oder installiere ihn in einer Shell:

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Wenn du das Repository geklont hast, kannst du stattdessen deinen Klon als Marktplatz nehmen. Dann sind Mod und Bridge derselbe Code, und nach einem `git pull` bekommt jede neue Sitzung die neue Version:

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

Auf Benutzerebene installiert, wird der Mod in jeder Claude-Code-Sitzung geladen, aber nur in Sitzungen aktiv, die mit `SYMPHONY_TOKEN` gestartet wurden; überall sonst tut er nichts.

Aktualisieren: `claude plugin update symphony-bridge@symphony`. Deinstallieren: `claude plugin uninstall symphony-bridge@symphony`.

### 2. Die Bridge starten

Lade das Archiv für dein System aus dem [neuesten Release](https://github.com/feynmanzcys-bytes/symphony/releases/latest) herunter, entpacke es und starte das enthaltene Programm `symphony`:

| System | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple Silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Zum Beispiel unter Linux:

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

Unter Windows entpackst du das ZIP-Archiv und startest `symphony.exe`; per Doppelklick läuft es in einem eigenen Fenster, und wenn du das Fenster schließt, stoppt die Bridge.

Die Programme sind nicht signiert, daher kann der erste Start blockiert werden. macOS lehnt eine mit dem Browser heruntergeladene Kopie womöglich ab, weil es den Entwickler nicht überprüfen kann: Klick unter Systemeinstellungen → Datenschutz & Sicherheit auf „Dennoch öffnen“ oder führe einmal `xattr -d com.apple.quarantine symphony` aus. Unter Windows kann SmartScreen ebenfalls warnen: Klick auf „Weitere Informationen“ und dann auf „Trotzdem ausführen“. Die `checksums.txt` im Release enthält die SHA-256-Prüfsumme jedes Archivs.

Programm und Mod haben dieselbe Versionsnummer: `symphony -version` zeigt die des Programms, `claude plugin list` die des Mods. Zum Aktualisieren lädst du das neue Release herunter und führst `claude plugin update symphony-bridge@symphony` aus. [CHANGELOG.md](CHANGELOG.md) listet auf, was sich in jedem Release geändert hat.

Oder starte die Bridge aus dem Quellcode, mit Go 1.27 oder neuer:

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

Das Büro öffnet sich in deinem Browser unter <http://127.0.0.1:7788/> (`-open=false` schaltet das ab); <http://127.0.0.1:7788/approve> ist eine einfachere Seite nur für Freigaben. Startest du die Bridge erneut, während sie schon läuft, öffnet sich nur das Büro.

| Flag | Standard | Wozu |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Adresse, auf der die Bridge lauscht. Die Seite hat noch keine Anmeldung, also binde die Bridge nur an diesen Rechner |
| `-data` | `$SYMPHONY_DATA`, sonst `symphony` in deinem Konfigurationsordner | Wo Konten (mit ihren Tokens) und der Nachrichtenverlauf liegen |
| `-terminal` | `auto` | Terminal für über die Seite gestartete Sitzungen: `auto`, `tmux` oder `window` |
| `-claude` | `claude` | Die ausführbare Datei von claude |
| `-lang` | deine Systemsprache | Sprache der Bridge-Meldungen |
| `-mod` | leer | Für die Entwicklung: gibt Sitzungen per `--plugin-dir` den Mod aus diesem Ordner mit, statt der installierten Kopie |
| `-web` | leer | Für die Entwicklung: liefert die Seiten aus diesem Ordner aus, sodass Änderungen nach dem Neuladen sichtbar sind |
| `-open` | `true` | Öffnet das Büro im Browser, sobald die Bridge läuft; läuft unter `-addr` schon eine Bridge, wird nur ihr Büro geöffnet |
| `-autostart` | — | `on`: Startet die Bridge bei jeder Anmeldung, mit den übrigen angegebenen Optionen, und startet sie gleich; `off`: macht das rückgängig |
| `-background` | — | Startet die Bridge im Hintergrund, losgelöst vom Terminal und mit Log in `bridge.log` im Datenordner, und kehrt zurück, sobald sie antwortet |
| `-version` | — | Gibt die Version aus und beendet sich |

**Beim Anmelden starten**: `symphony -autostart on` lässt die Bridge bei jeder Anmeldung starten, mit den gleich mitgegebenen Optionen (etwa `-addr`), und startet sie sofort; `symphony -autostart off` macht das rückgängig. Unter Linux ist das ein systemd-Benutzerdienst (`journalctl --user -u symphony` zeigt sein Log), unter macOS ein launchd-Agent (Log in `~/Library/Logs/symphony.log`), unter Windows ein Wert unter dem Run-Schlüssel der Registry (die Bridge läuft in einem Konsolenfenster; schließt du es, ist sie bis zur nächsten Anmeldung aus). Dafür braucht es das Programm symphony selbst, nicht `go run`.

### 3. Sitzungen starten

**Über die Seite**: Klick auf das „+“ an einem leeren Platz, gib einen Namen ein und wähle Ordner und Optionen. Die Bridge öffnet ein Terminal, in dem `claude` läuft; bei tmux heißt die tmux-Sitzung `sym-<id>`, und mit `tmux attach -t sym-<id>` kommst du hin.

**Aus einem Terminal**: Leg ein Konto an, um ein Token zu bekommen, und starte `claude` damit:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # die Antwort enthält das Token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Empfohlen ist der Berechtigungsmodus `manual`: Im Modus `auto` wird bei den meisten Aktionen niemand gefragt, also kommen kaum Freigaben auf der Seite an. Läuft die Bridge nicht unter der Standardadresse, setz zusätzlich `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` legt die Sprache der Mod-Meldungen fest.

Antwortet unter der Adresse der Bridge auf diesem Rechner nichts, startet die Sitzung die Bridge selbst mit `symphony -background` (wenn `symphony` in deinem PATH liegt), und die Bridge läuft nach dem Ende der Sitzung weiter. Setz `SYMPHONY_DATA`, wenn deine Konten nicht im Standard-Datenordner liegen.

## Konten und Plätze

- Eine Identität in symphony besteht aus einem Namen, einem Token und einem festen Platz. Ein Konto kann immer nur von einer Sitzung zugleich genutzt werden; eine zweite Sitzung wird abgewiesen, und ihre Freigaben bleiben im Terminal.
- Claude-Logins werden hier nicht verwaltet: Alle Sitzungen können sich ein Claude-Login teilen.
- Die Daten liegen in `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) oder `%AppData%\symphony` (Windows). `accounts.json` enthält die Tokens und ist nur für den Besitzer lesbar (0600); unter Windows übernehmen das die Zugriffsrechte des Benutzerordners.

## Sicherheit

- Die Bridge nimmt nur Anfragen an, die an diesen Rechner gerichtet sind, und lehnt schreibende Anfragen ab, die eine andere Website über deinen Browser schickt: Die Seite kann Sitzungen starten, also Befehle auf diesem Rechner ausführen.
- Die Seite hat noch keine Anmeldung, also binde die Bridge **nicht** an eine andere Adresse als die dieses Rechners.

## Plattformen

| System | Terminal für über die Seite gestartete Sitzungen | Getestet |
| --- | --- | --- |
| Linux | tmux, falls installiert, sonst ein grafisches Terminal | Vollständige End-to-End-Tests |
| macOS | Standardmäßig Terminal.app; `-terminal tmux` für tmux | Cross-Kompilierung und Unit-Tests; `-autostart` auch End-to-End getestet |
| Windows | Ein neues Konsolenfenster | Nur Cross-Kompilierung und Unit-Tests |

Nur tmux kann „trust this folder“ für dich bestätigen. Bei einem Terminalfenster musst du für die erste Sitzung in einem neuen Ordner einmal in diesem Fenster klicken.

## Entwicklung

```bash
go test -race ./...                         # Unit-Tests für Bridge und Übersetzungen
claude plugin test mods/symphony-bridge     # Unit-Tests für den Mod
claude plugin validate .                    # Marktplatz und Mod prüfen
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # End-to-End: startet echte Sitzungen und verbraucht Tokens
```

- Am Mod arbeiten: `go run ./cmd/bridge -mod mods/symphony-bridge`, dann bekommen über die Seite gestartete Sitzungen deine Kopie des Mods per `--plugin-dir`; Sitzungen, die du von Hand startest, gibst du `--plugin-dir mods/symphony-bridge` selbst mit.
- An den Seiten arbeiten: `go run ./cmd/bridge -web web`, dann zeigt ein Neuladen deine Änderungen.
- Das Büro ohne echte Sitzungen ansehen: Starte eine zweite Bridge und füll sie mit ausgedachter Aktivität: `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`, dann `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Übersetzungen: Jede Sprache ist eine Datei, `web/locales/<lang>.json` für die Seiten und `internal/i18n/locales/<lang>.json` für die Bridge, mit denselben Schlüsseln wie `en.json`; die paar Meldungen des Mods stehen in `mods/symphony-bridge/hooks/messages.ts`. Die Unit-Tests prüfen, dass jede Sprache alle Schlüssel und dieselben `{Platzhalter}` hat, und der End-to-End-Test `TestI18n` öffnet die Seiten in jeder Sprache.
- Die End-to-End-Tests brauchen claude, tmux und chromium, laufen also unter Linux und unter macOS mit tmux. Standardmäßig nutzen sie den Mod aus diesem Repository; mit `SYMPHONY_E2E_MOD=installed` nehmen sie den installierten Mod. Screenshots landen in `e2e/artifacts/`.
- Releases: Versieh einen Commit auf main mit seiner Version als Tag und pushe den Tag (`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`); GitHub Actions baut dann das Programm für alle Plattformen und veröffentlicht ein Release (`.github/workflows/release.yml`). Der Tag muss die Version des Mods in `plugin.json` sein. `.github/release.sh v0.3.0 dist` baut dieselben Archive lokal. Jedes Release braucht einen Abschnitt in `CHANGELOG.md`, der den Anfang seiner Release Notes bildet.

| Ordner | Was drin ist |
| --- | --- |
| `mods/symphony-bridge/` | Der Claude-Code-Mod (TypeScript) |
| `.claude-plugin/marketplace.json` | Der Plugin-Marktplatz, den `/plugin install` liest |
| `cmd/bridge/`, `internal/bridge/` | Die Bridge |
| `internal/i18n/` | Spracherkennung und die Übersetzungen der Bridge |
| `web/` | Büro- und Freigabeseite samt Übersetzungen, in die Bridge eingebaut |
| `cmd/demo/` | Ausgedachte Aktivität fürs Büro |
| `e2e/` | End-to-End-Tests |

## Lizenz

[MIT](LICENSE)
