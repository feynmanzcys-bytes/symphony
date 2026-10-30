# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | **Deutsch** | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Setz ein Team aus Claude-Code-Sitzungen in ein Pixel-Art-Büro: Du siehst auf einen Blick, wer gerade arbeitet und wer auf dich wartet, und erledigst Berechtigungsanfragen und Fragen per AskUserQuestion mit einem Klick in einem Pop-up im Browser.

![Das Büro](docs/office.png)

## Was es kann

- **Freigaben im Browser**: Berechtigungsanfragen (Befehle ausführen, Dateien bearbeiten) und Fragen per AskUserQuestion landen auf der Webseite. Läuft die Bridge nicht oder kennt sie das Konto nicht, fällt die Sitzung wie gewohnt aufs Terminal zurück und bleibt nie hängen.
- **Ein Pixel-Art-Büro**: Jede Sitzung ist ein Mitarbeiter mit festem Platz. Grün ist frei, Gelb bei der Arbeit, Rot wartet auf deine Freigabe oder Antwort, Grau hat Feierabend. Wer nichts zu tun hat, bleibt erst am Platz und hängt am Handy oder zockt, dann zieht er los: Fernsehen auf dem Lounge-Sofa, Kicker, Fangen, ein Plausch am runden Tisch, Kaffee, ein Blick aus dem Fenster; nach einer Weile folgt ein Nickerchen, meist versteckt im Meetingraum. Die Größe der Figur richtet sich nach dem Modell, ihre Muskeln nach dem Denkaufwand.
- **Jeden finden und zurückrufen**: Klick auf einen freien Mitarbeiter, und er geht an seinen Platz zurück (und bleibt dort bis zu seiner nächsten Aufgabe); das Suchfeld neben dem Titel findet Mitarbeiter nach Name oder Ordner (mit `/` springst du hinein).
- **Nachrichten zwischen Sitzungen**: Wenn sich Sitzungen gegenseitig Nachrichten schicken (SendMessage), fliegt ein Briefumschlag durchs Büro, und der Nachrichtenverlauf hält sie fest.
- **Sitzungen über die Seite starten**: Klick auf das „+“ an einem leeren Platz, um einen neuen Mitarbeiter einzustellen, und wähle Ordner, Modell, Denkaufwand und Berechtigungsmodus; klick bei einem Mitarbeiter, der Feierabend hat, auf „An die Arbeit“, und er kehrt an seinen alten Platz zurück.
- **10 Sprachen**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) und Русский. Die Seite richtet sich nach der Sprache deines Browsers und hat oben links eine Sprachauswahl; die Meldungen der Bridge folgen deiner Systemsprache (oder `-lang`), die des Mods der Seite, die die Sitzung gestartet hat.
- **Nur lokal**: Die Bridge ist ein einzelnes Go-Programm (nur Standardbibliothek), die Seiten sind reines JavaScript, und nichts hängt von einem Drittanbieterdienst ab.

![Freigaben im Browser](docs/approve.png)

## Voraussetzungen

- Claude Code 2.1.314 (mit dieser Version getestet). Der Mod nutzt die Function Hooks von Claude Code, die noch eine Early-Access-API sind; führe also nach einem Update von Claude Code die End-to-End-Tests aus.
- Go 1.27 oder neuer, um die Bridge zu starten.
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

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

Unter <http://127.0.0.1:7788/> findest du das Büro. <http://127.0.0.1:7788/approve> ist eine einfachere Seite nur für Freigaben.

| Flag | Standard | Wozu |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Adresse, auf der die Bridge lauscht. Die Seite hat noch keine Anmeldung, also binde die Bridge nur an diesen Rechner |
| `-data` | `symphony` in deinem Konfigurationsordner | Wo Konten (mit ihren Tokens) und der Nachrichtenverlauf liegen |
| `-terminal` | `auto` | Terminal für über die Seite gestartete Sitzungen: `auto`, `tmux` oder `window` |
| `-claude` | `claude` | Die ausführbare Datei von claude |
| `-lang` | deine Systemsprache | Sprache der Bridge-Meldungen |
| `-mod` | leer | Für die Entwicklung: gibt Sitzungen per `--plugin-dir` den Mod aus diesem Ordner mit, statt der installierten Kopie |
| `-web` | leer | Für die Entwicklung: liefert die Seiten aus diesem Ordner aus, sodass Änderungen nach dem Neuladen sichtbar sind |

### 3. Sitzungen starten

**Über die Seite**: Klick auf das „+“ an einem leeren Platz, gib einen Namen ein und wähle Ordner und Optionen. Die Bridge öffnet ein Terminal, in dem `claude` läuft; bei tmux heißt die tmux-Sitzung `sym-<id>`, und mit `tmux attach -t sym-<id>` kommst du hin.

**Aus einem Terminal**: Leg ein Konto an, um ein Token zu bekommen, und starte `claude` damit:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # die Antwort enthält das Token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Empfohlen ist der Berechtigungsmodus `manual`: Im Modus `auto` wird bei den meisten Aktionen niemand gefragt, also kommen kaum Freigaben auf der Seite an. Läuft die Bridge nicht unter der Standardadresse, setz zusätzlich `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` legt die Sprache der Mod-Meldungen fest.

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
| macOS | Standardmäßig Terminal.app; `-terminal tmux` für tmux | Nur Cross-Kompilierung und Unit-Tests |
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
