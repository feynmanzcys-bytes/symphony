# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | **Français** | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Réunissez une équipe de sessions Claude Code dans un bureau en pixel art : voyez d’un coup d’œil qui travaille et qui vous attend, parlez à chacun dans sa propre fenêtre, et traitez-y en un clic les demandes d’autorisation et les questions AskUserQuestion.

![Le bureau](docs/office.png)

## Fonctionnalités

- **Chaque session comme une discussion** : cliquez sur un employé et sa fenêtre s’ouvre au milieu de la page : en haut qui il est, en dessous la session sous forme de discussion : les instructions, la réflexion du modèle (repliée, « Réflexion… » en orange tant qu’elle dure), ses réponses au fil de l’eau, les appels d’outils, et les messages des autres sessions en violet. Écrivez en bas et le texte arrive à la session comme votre instruction ; le bouton Arrêter l’interrompt.
- **Approbations dans le navigateur** : les demandes d’autorisation (exécuter des commandes, modifier des fichiers) et les questions AskUserQuestion remontent sous forme de cartes par-dessus la zone de saisie de la fenêtre de cet employé, et le compteur rouge de la barre du haut vous y amène. Si le bridge ne tourne pas ou ne connaît pas le compte, la session revient au terminal comme d’habitude et ne reste jamais bloquée.
- **Un bureau en pixel art** : chaque session est un employé à un poste fixe. Vert : disponible ; jaune : occupé ; rouge : attend votre autorisation ou votre réponse ; gris : hors service. Les employés désœuvrés traînent d’abord à leur poste avec leur téléphone ou leurs jeux, puis partent se promener : télé sur le canapé de l’espace détente, baby-foot, partie de chat, discussion à la table ronde, café, coup d’œil par la fenêtre ; au bout d’un moment, ils font la sieste, souvent cachés dans la salle de réunion. La taille du personnage dépend du modèle, et ses muscles de l’effort de réflexion. Le bureau est un étage de l’immeuble d’une grande entreprise et remplit la fenêtre : l’open space au centre, de petites salles de réunion de 3 à 5 places avec leur plaque à gauche, la salle de réunion et l’espace détente à droite, et des ascenseurs pour arriver et repartir. La barre du haut règle aussi la taille des pixels, en quatre crans : le même bureau, dessiné avec des pixels plus fins et plus de détails à chaque cran.
- **Retrouver quelqu’un et le rappeler** : cliquez sur un employé disponible et il retourne à son poste (il y reste jusqu’à sa prochaine tâche) ; le champ de recherche de la barre du haut trouve les employés par nom ou par dossier (touche `/` pour y aller), et un clic sur un compteur à côté (disponible, occupé, en attente, hors service) fait ressortir tous les employés dans cet état. L’onglet sous la barre la range vers le haut et la fait redescendre.
- **Messages entre sessions** : quand des sessions s’envoient des messages (SendMessage), une enveloppe traverse le bureau, et le journal des messages les conserve.
- **Démarrer des sessions depuis la page, sans qu’aucune fenêtre ne s’ouvre** : cliquez sur le « + » d’un poste libre pour embaucher un nouvel employé, puis choisissez le dossier, le modèle, l’effort de réflexion et le mode d’autorisation (un nouvel employé démarre avec tout autorisé, Opus et le maximum de réflexion) ; cliquez sur « Au travail ! » dans la fenêtre d’un employé hors service pour le faire revenir à son poste. La session tourne sur un terminal à elle, à l’intérieur du bridge : rien ne s’ouvre et rien ne vous prend le focus ; « Ouvrir le terminal » dans la fenêtre de l’employé montre ce terminal quand vous le voulez, et le fermer ne change rien.
- **10 langues** : English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) et Русский. La page suit la langue de votre navigateur et propose un sélecteur de langue dans la barre du haut ; les messages du bridge suivent la langue de votre système (ou `-lang`), et ceux du mod suivent la page qui a démarré la session.
- **Tout en local** : le bridge est un seul programme Go (bibliothèque standard uniquement), les pages sont en JavaScript pur, et rien ne dépend d’un service tiers.

![Approbations dans le navigateur](docs/approve.png)

## Prérequis

- Claude Code 2.1.314 (testé avec cette version). Le mod utilise les function hooks de Claude Code, une API encore en accès anticipé : après chaque mise à jour de Claude Code, relancez les tests de bout en bout.
- Le bridge existe en programme prêt à l’emploi pour Linux, macOS et Windows ; Go 1.27 ou plus récent n’est nécessaire que pour le lancer depuis les sources.
- Rien d’autre : les sessions démarrées depuis la page tournent dans le bridge, sur toutes les plateformes.

## Installation

### 1. Installer le mod

Dans Claude Code, tapez :

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Appuyez sur `y` pour ajouter la marketplace, puis choisissez une portée (la portée utilisateur, proposée par défaut). Ou installez-le depuis un shell :

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Si vous avez cloné le dépôt, vous pouvez plutôt utiliser votre clone comme marketplace. Le mod et le bridge viennent alors du même code, et après un `git pull`, chaque nouvelle session reçoit la nouvelle version :

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

Installé avec la portée utilisateur, le mod se charge dans toutes les sessions Claude Code, mais il n’agit que dans celles lancées avec `SYMPHONY_TOKEN` ; partout ailleurs, il ne fait rien.

Mise à jour : `claude plugin update symphony-bridge@symphony`. Désinstallation : `claude plugin uninstall symphony-bridge@symphony`.

### 2. Démarrer le bridge

Téléchargez l’archive pour votre système depuis la [dernière version](https://github.com/feynmanzcys-bytes/symphony/releases/latest), décompressez-la et lancez le programme `symphony` qu’elle contient :

| Système | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Par exemple, sous Linux :

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

Sous Windows, décompressez le zip et lancez `symphony.exe` ; d’un double-clic, il s’ouvre dans sa propre fenêtre, et fermer cette fenêtre arrête le bridge.

Les programmes ne sont pas signés : le premier lancement peut être bloqué. macOS peut refuser une copie téléchargée avec un navigateur, faute de pouvoir vérifier le développeur : cliquez sur « Ouvrir quand même » dans Réglages Système → Confidentialité et sécurité, ou lancez une fois `xattr -d com.apple.quarantine symphony`. Sous Windows, SmartScreen peut aussi vous avertir : cliquez sur « Informations complémentaires », puis sur « Exécuter quand même ». Le fichier `checksums.txt` de la version donne le SHA-256 de chaque archive.

Le programme et le mod partagent le même numéro de version : `symphony -version` affiche celui du programme, `claude plugin list` celui du mod. Pour mettre à jour, téléchargez la nouvelle version et lancez `claude plugin update symphony-bridge@symphony`. [CHANGELOG.md](CHANGELOG.md) liste ce qui a changé dans chaque version.

Ou lancez le bridge depuis les sources, avec Go 1.27 ou plus récent :

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

Le bureau s’ouvre dans votre navigateur à l’adresse <http://127.0.0.1:7788/> (`-open=false` le désactive) ; <http://127.0.0.1:7788/approve> est une page plus simple, réservée aux approbations. Relancer le bridge alors qu’il tourne déjà ouvre simplement le bureau.

| Option | Par défaut | Rôle |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Adresse d’écoute. La page n’a pas encore d’authentification : n’écoutez que sur cette machine |
| `-data` | `$SYMPHONY_DATA`, sinon `symphony` dans votre dossier de configuration | Où sont conservés les comptes (avec leurs tokens) et le journal des messages |
| `-claude` | `claude` | L’exécutable claude |
| `-lang` | la langue de votre système | Langue des messages du bridge |
| `-mod` | vide | Pour le développement : donne aux sessions le mod de ce dossier avec `--plugin-dir`, à la place de la copie installée |
| `-web` | vide | Pour le développement : sert les pages depuis ce dossier, pour voir les modifications après un rechargement |
| `-open` | `true` | Ouvre le bureau dans le navigateur une fois le bridge démarré ; si un bridge tourne déjà sur `-addr`, ouvre simplement son bureau |
| `-autostart` | — | `on` : démarre le bridge à chaque ouverture de session, avec les autres options données, et le démarre tout de suite ; `off` : annule cela |
| `-background` | — | Démarre le bridge en arrière-plan, détaché du terminal et journalisant dans `bridge.log` du dossier de données, puis rend la main dès qu’il répond |
| `-version` | — | Affiche la version et quitte |

**Le lancer à l’ouverture de session** : `symphony -autostart on` fait démarrer le bridge à chaque ouverture de session, avec les options données en même temps (par exemple `-addr`), et le démarre tout de suite ; `symphony -autostart off` annule cela. Sous Linux, c’est un service utilisateur systemd (`journalctl --user -u symphony` affiche son journal), sous macOS un agent launchd (journal dans `~/Library/Logs/symphony.log`), sous Windows une valeur sous la clé Run du registre (le bridge tourne dans une fenêtre de console ; la fermer l’arrête jusqu’à la prochaine ouverture de session). Il faut le programme symphony lui-même, pas `go run`.

### 3. Démarrer des sessions

**Depuis la page** : cliquez sur le « + » d’un poste libre, saisissez un nom, puis choisissez le dossier et les options. Le bridge lance `claude` sur un terminal à lui, à l’intérieur du bridge : aucune fenêtre ne s’ouvre. La fenêtre de l’employé montre la session sous forme de discussion, et son bouton « Ouvrir le terminal » ouvre une fenêtre de terminal reliée à la session (`symphony attach <id>` fait de même depuis n’importe quel terminal ; `Ctrl-]` ou la fermeture de la fenêtre détache, et la session continue). Les sessions sont des processus enfants du bridge : arrêter le bridge les termine.

**Depuis un terminal** : créez un compte pour obtenir un token, puis lancez `claude` avec ce token :

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # la réponse contient le token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Le mode d’autorisation `manual` est recommandé : en mode `auto`, la plupart des actions ne demandent rien à personne, donc peu d’approbations arrivent jusqu’à la page. Si le bridge n’est pas à l’adresse par défaut, définissez aussi `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` choisit la langue des messages du mod.

Quand rien ne répond à l’adresse du bridge sur cette machine, la session démarre elle-même le bridge avec `symphony -background` (si `symphony` est dans votre PATH), et le bridge continue de tourner après la fin de la session. Définissez `SYMPHONY_DATA` si vos comptes ne sont pas dans le dossier de données par défaut.

## Comptes et postes

- Dans symphony, une identité, c’est un nom, un token et un poste fixe. Un compte ne peut servir qu’à une session à la fois ; une deuxième session est refusée et ses approbations restent dans le terminal.
- La connexion à Claude n’est pas gérée ici : toutes les sessions peuvent partager le même identifiant Claude.
- Les données sont stockées dans `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) ou `%AppData%\symphony` (Windows). `accounts.json` contient les tokens et n’est lisible que par son propriétaire (0600) ; sous Windows, ce sont les droits d’accès du dossier utilisateur qui le protègent.

## Sécurité

- Le bridge n’accepte que les requêtes adressées à cette machine, et refuse les requêtes d’écriture qu’un autre site enverrait via votre navigateur : la page peut démarrer des sessions, c’est-à-dire exécuter des commandes sur cette machine.
- La page n’a pas encore d’authentification : **ne faites pas** écouter le bridge sur une autre adresse que celle de cette machine.

## Plateformes

| Système | Sessions démarrées depuis la page | « Ouvrir le terminal » | Tests |
| --- | --- | --- | --- |
| Linux | Un pseudo-terminal dans le bridge | Un terminal graphique (x-terminal-emulator, gnome-terminal, konsole, xterm) qui lance `symphony attach` | Tests de bout en bout complets |
| macOS | Un pseudo-terminal dans le bridge | Terminal.app lançant `symphony attach` | Tests de bout en bout complets |
| Windows | Une pseudo-console (ConPTY) dans le bridge | Une nouvelle fenêtre de console qui lance `symphony attach` | Compilation croisée et tests unitaires uniquement |

Le bridge choisit « faire confiance à ce dossier » à votre place, sur toutes les plateformes.

## Développement

```bash
go test -race ./...                         # tests unitaires du bridge et des traductions
claude plugin test mods/symphony-bridge     # tests unitaires du mod
claude plugin validate .                    # vérifie la marketplace et le mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # bout en bout : démarre de vraies sessions et consomme des tokens
```

- Travailler sur le mod : `go run ./cmd/bridge -mod mods/symphony-bridge`, et les sessions démarrées depuis la page reçoivent votre copie du mod via `--plugin-dir` ; pour les sessions que vous lancez à la main, ajoutez vous-même `--plugin-dir mods/symphony-bridge`.
- Travailler sur les pages : `go run ./cmd/bridge -web web`, et un rechargement affiche vos modifications.
- Voir le bureau sans vraies sessions : démarrez un second bridge et remplissez-le d’activité fictive : `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`, puis `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Traductions : chaque langue tient dans un fichier, `web/locales/<lang>.json` pour les pages et `internal/i18n/locales/<lang>.json` pour le bridge, avec les mêmes clés que `en.json` ; les quelques messages du mod se trouvent dans `mods/symphony-bridge/hooks/messages.ts`. Les tests unitaires vérifient que chaque langue a toutes les clés et les mêmes `{variables}`, et le test de bout en bout `TestI18n` ouvre les pages dans chaque langue.
- Les tests de bout en bout ont besoin de claude, tmux (pour les sessions que les tests pilotent eux-mêmes) et Chromium ou Chrome : ils tournent donc sous Linux et macOS. Par défaut, ils utilisent le mod de ce dépôt ; définissez `SYMPHONY_E2E_MOD=installed` pour utiliser le mod installé. Les captures d’écran sont enregistrées dans `e2e/artifacts/`.
- Versions : étiquetez un commit de main avec sa version et poussez l’étiquette (`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`) ; GitHub Actions compile alors le programme pour toutes les plateformes et publie une version (`.github/workflows/release.yml`). L’étiquette doit être la version du mod dans `plugin.json`. `.github/release.sh v0.3.0 dist` produit les mêmes archives en local. Chaque version doit avoir sa section dans `CHANGELOG.md`, qui ouvre ses notes.

| Dossier | Contenu |
| --- | --- |
| `mods/symphony-bridge/` | Le mod Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | La marketplace de plugins que lit `/plugin install` |
| `cmd/bridge/`, `internal/bridge/` | Le bridge |
| `internal/i18n/` | La détection de la langue et les traductions du bridge |
| `web/` | Les pages du bureau et des approbations, avec leurs traductions, intégrées au bridge |
| `cmd/demo/` | De l’activité fictive pour le bureau |
| `e2e/` | Les tests de bout en bout |

## Licence

[MIT](LICENSE)
