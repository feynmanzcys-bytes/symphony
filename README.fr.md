# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | **Français** | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Réunissez une équipe de sessions Claude Code dans un bureau en pixel art : voyez d’un coup d’œil qui travaille et qui vous attend, et traitez en un clic, dans une fenêtre de la page web, les demandes d’autorisation et les questions AskUserQuestion.

![Le bureau](docs/office.png)

## Fonctionnalités

- **Approbations dans le navigateur** : les demandes d’autorisation (exécuter des commandes, modifier des fichiers) et les questions AskUserQuestion arrivent dans la page web. Si le bridge ne tourne pas ou ne connaît pas le compte, la session revient au terminal comme d’habitude et ne reste jamais bloquée.
- **Un bureau en pixel art** : chaque session est un employé à un poste fixe. Vert : disponible ; jaune : occupé ; rouge : attend votre autorisation ou votre réponse ; gris : hors service. Les employés désœuvrés traînent d’abord à leur poste avec leur téléphone ou leurs jeux, puis partent se promener : télé sur le canapé de l’espace détente, baby-foot, partie de chat, discussion à la table ronde, café, coup d’œil par la fenêtre ; au bout d’un moment, ils font la sieste, souvent cachés dans la salle de réunion. La taille du personnage dépend du modèle, et ses muscles de l’effort de réflexion.
- **Retrouver quelqu’un et le rappeler** : cliquez sur un employé disponible et il retourne à son poste (il y reste jusqu’à sa prochaine tâche) ; le champ de recherche à côté du titre trouve les employés par nom ou par dossier (touche `/` pour y aller).
- **Messages entre sessions** : quand des sessions s’envoient des messages (SendMessage), une enveloppe traverse le bureau, et le journal des messages les conserve.
- **Démarrer des sessions depuis la page** : cliquez sur le « + » d’un poste libre pour embaucher un nouvel employé, puis choisissez le dossier, le modèle, l’effort de réflexion et le mode d’autorisation ; cliquez sur « Au travail ! » dans la fiche d’un employé hors service pour le faire revenir à son poste.
- **10 langues** : English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) et Русский. La page suit la langue de votre navigateur et propose un sélecteur de langue en haut à gauche ; les messages du bridge suivent la langue de votre système (ou `-lang`), et ceux du mod suivent la page qui a démarré la session.
- **Tout en local** : le bridge est un seul programme Go (bibliothèque standard uniquement), les pages sont en JavaScript pur, et rien ne dépend d’un service tiers.

![Approbations dans le navigateur](docs/approve.png)

## Prérequis

- Claude Code 2.1.314 (testé avec cette version). Le mod utilise les function hooks de Claude Code, une API encore en accès anticipé : après chaque mise à jour de Claude Code, relancez les tests de bout en bout.
- Go 1.27 ou plus récent, pour faire tourner le bridge.
- Pour démarrer des sessions depuis la page : tmux est recommandé sous Linux et macOS.

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

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

Ouvrez <http://127.0.0.1:7788/> pour voir le bureau. <http://127.0.0.1:7788/approve> est une page plus simple, réservée aux approbations.

| Option | Par défaut | Rôle |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Adresse d’écoute. La page n’a pas encore d’authentification : n’écoutez que sur cette machine |
| `-data` | `symphony` dans votre dossier de configuration | Où sont conservés les comptes (avec leurs tokens) et le journal des messages |
| `-terminal` | `auto` | Terminal des sessions démarrées depuis la page : `auto`, `tmux` ou `window` |
| `-claude` | `claude` | L’exécutable claude |
| `-lang` | la langue de votre système | Langue des messages du bridge |
| `-mod` | vide | Pour le développement : donne aux sessions le mod de ce dossier avec `--plugin-dir`, à la place de la copie installée |
| `-web` | vide | Pour le développement : sert les pages depuis ce dossier, pour voir les modifications après un rechargement |

### 3. Démarrer des sessions

**Depuis la page** : cliquez sur le « + » d’un poste libre, saisissez un nom, puis choisissez le dossier et les options. Le bridge ouvre un terminal qui lance `claude` ; avec tmux, la session s’appelle `sym-<id>`, et `tmux attach -t sym-<id>` vous y amène.

**Depuis un terminal** : créez un compte pour obtenir un token, puis lancez `claude` avec ce token :

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # la réponse contient le token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Le mode d’autorisation `manual` est recommandé : en mode `auto`, la plupart des actions ne demandent rien à personne, donc peu d’approbations arrivent jusqu’à la page. Si le bridge n’est pas à l’adresse par défaut, définissez aussi `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` choisit la langue des messages du mod.

## Comptes et postes

- Dans symphony, une identité, c’est un nom, un token et un poste fixe. Un compte ne peut servir qu’à une session à la fois ; une deuxième session est refusée et ses approbations restent dans le terminal.
- La connexion à Claude n’est pas gérée ici : toutes les sessions peuvent partager le même identifiant Claude.
- Les données sont stockées dans `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) ou `%AppData%\symphony` (Windows). `accounts.json` contient les tokens et n’est lisible que par son propriétaire (0600) ; sous Windows, ce sont les droits d’accès du dossier utilisateur qui le protègent.

## Sécurité

- Le bridge n’accepte que les requêtes adressées à cette machine, et refuse les requêtes d’écriture qu’un autre site enverrait via votre navigateur : la page peut démarrer des sessions, c’est-à-dire exécuter des commandes sur cette machine.
- La page n’a pas encore d’authentification : **ne faites pas** écouter le bridge sur une autre adresse que celle de cette machine.

## Plateformes

| Système | Terminal des sessions démarrées depuis la page | Tests |
| --- | --- | --- |
| Linux | tmux s’il est installé, sinon un terminal graphique | Tests de bout en bout complets |
| macOS | Terminal.app par défaut ; `-terminal tmux` pour tmux | Compilation croisée et tests unitaires uniquement |
| Windows | Une nouvelle fenêtre de console | Compilation croisée et tests unitaires uniquement |

Seul tmux peut choisir « faire confiance à ce dossier » à votre place. Avec une fenêtre de terminal, la première session dans un nouveau dossier demande un clic dans cette fenêtre.

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
- Les tests de bout en bout ont besoin de claude, tmux et chromium : ils tournent donc sous Linux, et sous macOS avec tmux. Par défaut, ils utilisent le mod de ce dépôt ; définissez `SYMPHONY_E2E_MOD=installed` pour utiliser le mod installé. Les captures d’écran sont enregistrées dans `e2e/artifacts/`.

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
