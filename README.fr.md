# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | **Français** | [Deutsch](README.de.md) | [Português (Brasil)](README.pt-BR.md) | [Русский](README.ru.md)

Réunissez une équipe de sessions Claude Code dans un bureau en pixel art : voyez d’un coup d’œil qui travaille et qui vous attend, parlez à chacun dans sa propre fenêtre, et traitez-y en un clic les demandes d’autorisation et les questions AskUserQuestion.

![Le bureau](docs/office.png)

## Installation

Le plus rapide : ouvrez Claude Code et collez

> Installe-moi https://github.com/feynmanzcys-bytes/symphony

Claude Code lit ce README et fait tout ce que décrit la section suivante. Pour installer à la main, suivez les mêmes étapes.

### Pour les agents IA : la procédure d’installation

Vous lisez ceci parce qu’un utilisateur vous a demandé d’installer symphony. Faites les étapes ci-dessous dans l’ordre, dans un shell, et terminez par le compte rendu de l’étape 6. Ne demandez rien à l’utilisateur sauf si une étape le prévoit ; faites vous-même les choix courants et listez-les dans le compte rendu.

symphony a deux parties et a besoin des deux :

1. **Le programme `symphony`** (le bridge) : un seul binaire, un serveur web local à l’adresse `http://127.0.0.1:7788/` qui sert la page du bureau et fait tourner les sessions. Il doit être dans le `PATH` de l’utilisateur, sous le nom `symphony`, car le mod le démarre avec `symphony -background` quand une session ne trouve aucun bridge en marche.
2. **Le plugin `symphony-bridge`** (le mod) : un plugin Claude Code issu de la marketplace de ce dépôt, installé avec la portée utilisateur. Il n’agit que dans les sessions lancées avec `SYMPHONY_TOKEN` et ne fait rien dans les autres.

#### Étape 1 : vérifier la machine

```bash
uname -sm                 # Linux/macOS : choisit l’archive ci-dessous (x86_64 → amd64 ; arm64/aarch64 → arm64)
claude --version          # il faut Claude Code 2.1.x (testé avec 2.1.316)
git --version             # seulement pour les tâches en équipe ; s’il manque, le noter dans le compte rendu sans s’arrêter
symphony -version         # déjà installé ? affiche "symphony v0.8.0 (darwin/arm64)"
curl -s http://127.0.0.1:7788/api/state | head -c 40   # un bridge tourne déjà ? affiche {"type":"snapshot",...
claude plugin list --json # le mod est déjà installé ? cherchez "id":"symphony-bridge@symphony"
```

Sous Windows, utilisez PowerShell : `$env:PROCESSOR_ARCHITECTURE` vaut `AMD64` ou `ARM64`, et `Invoke-WebRequest http://127.0.0.1:7788/api/state` remplace curl.

Si `claude` manque, arrêtez-vous et dites à l’utilisateur d’installer d’abord Claude Code. Si le programme et le mod sont déjà tous les deux en place, passez plutôt à « Mise à jour » ci-dessous.

#### Étape 2 : récupérer le programme

Prenez la première option qui fonctionne :

- **A. Avec `gh`** (fonctionne tant que ce dépôt est privé, à condition que `gh auth status` indique une connexion) :
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. Avec `curl`** (seulement une fois le dépôt public ; un 404 veut dire qu’il faut passer par A ou C) :
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. Depuis les sources** (Go 1.27 ou plus récent ; le clone resservira à l’étape 4) :
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  Compilez le binaire ; `go run` ne convient pas, car `-autostart` refuse un build `go run`.

| Système | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Chaque archive contient le programme (`symphony`, ou `symphony.exe` sous Windows), `LICENSE` et `README.md`. Le fichier `checksums.txt` de la version donne le SHA-256 de chaque archive (`sha256sum -c --ignore-missing checksums.txt`, ou `shasum -a 256 -c` sous macOS).

#### Étape 3 : le mettre dans le PATH

Choisissez un dossier déjà présent dans le `PATH` de l’utilisateur et accessible en écriture sans `sudo` : `~/.local/bin`, `/usr/local/bin` ou `/opt/homebrew/bin`, dans cet ordre. Si aucun n’existe, créez `~/.local/bin`, ajoutez-le au fichier rc du shell (`~/.zshrc` ou `~/.bashrc`) et signalez-le dans le compte rendu.

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS : le binaire n’est pas signé
symphony -version          # affiche "symphony v0.8.0 (linux/amd64)" : la version et la plateforme
```

Sous Windows : `Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`, puis ajoutez ce dossier au `Path` de l’utilisateur (`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`) et ouvrez un nouveau shell. SmartScreen peut afficher un avertissement au premier lancement : « Informations complémentaires », puis « Exécuter quand même ».

Si votre propre shell ne trouve pas `symphony` après cela (son `PATH` a été lu avant votre modification), utilisez le chemin absolu pour la suite des étapes.

#### Étape 4 : installer le mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Cette commande ajoute le dépôt comme marketplace de plugins `symphony` et installe le mod avec la portée utilisateur, sans rien demander. Elle clone le dépôt avec git : tant que le dépôt est privé, il faut donc un accès git à GitHub. Si elle échoue pour cette raison, utilisez plutôt un clone comme marketplace (celui de l’étape 2 C, ou un nouveau) : `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. Avec un clone comme marketplace, les sessions lisent le mod dans ce dossier, et un `git pull` dans ce dossier le met à jour.

Vérifiez :

```bash
claude plugin list --json   # une entrée contient "id":"symphony-bridge@symphony", "scope":"user", "enabled":true
```

- `"scope":"project"` ou `"local"` : le mod ne se chargerait que dans un seul dossier. Lancez `claude plugin uninstall symphony-bridge@symphony`, puis réinstallez avec `--scope user`.
- `"enabled":false` : `claude plugin enable symphony-bridge@symphony`.

#### Étape 5 : démarrer le bridge

```bash
symphony -autostart on -open=false
```

Cette commande démarre le bridge maintenant et à chaque ouverture de session (un service utilisateur systemd sous Linux, un agent launchd sous macOS, la clé Run du registre sous Windows), et affiche

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

Si l’utilisateur a dit ne pas vouloir que le bridge démarre à l’ouverture de session, lancez plutôt `symphony -background -open=false` : cette commande démarre le bridge détaché du terminal et affiche `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log`. Les deux commandes répondent `a symphony bridge is already running at http://127.0.0.1:7788/` quand un bridge tourne déjà ; c’est normal, n’en démarrez pas un autre.

Vérifiez toute la chaîne avec une seule requête :

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

La réponse contient `"enabled":true` et `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.8.0"}` : le bridge tourne, il a trouvé `claude`, et le mod est installé là où les sessions le chargeront. Avec `"enabled":false`, `mod.code` dit ce qui ne va pas et `mod.fix` donne la commande qui le corrige : `mod_not_installed` ou `mod_project_scope` (retour à l’étape 4), `mod_disabled` (activez-le), `mod_check_failed` (`claude plugin list --json` a échoué, donc Claude Code est probablement trop ancien). `"mod":null` veut dire que le bridge n’a pas du tout trouvé `claude` dans son `PATH` (son journal le signale) : ajoutez-le au PATH, ou démarrez le bridge avec `-claude /path/to/claude`.

Le journal du bridge est `bridge.log` dans le dossier de données (`~/.config/symphony` sous Linux, `~/Library/Application Support/symphony` sous macOS, `%AppData%\symphony` sous Windows) ; démarré à l’ouverture de session, il journalise dans `journalctl --user -u symphony` sous Linux et dans `~/Library/Logs/symphony.log` sous macOS.

#### Étape 6 : rendre compte à l’utilisateur

Ouvrez le bureau dans le navigateur (`open http://127.0.0.1:7788/` sous macOS, `xdg-open` sous Linux, `start` sous Windows) et dites à l’utilisateur :

- Le bureau est à l’adresse `http://127.0.0.1:7788/`. Cliquez sur le « + » d’un poste libre pour embaucher le premier employé : choisissez un dossier, et une session Claude Code y démarre, à l’intérieur du bridge ; sa fenêtre dans la page est sa discussion.
- Seules les sessions démarrées depuis la page (ou à la main avec `SYMPHONY_TOKEN`, voir « Démarrer des sessions » plus bas) apparaissent dans le bureau. Les autres sessions Claude Code de l’utilisateur, y compris celle qui fait cette installation, n’y figurent pas.
- Ce qui a été installé et où : le chemin du programme, le mod avec la portée utilisateur, le bridge démarré à l’ouverture de session ou en arrière-plan, toute modification du `PATH`, et l’absence de `git` le cas échéant.
- Comment tout défaire : « Désinstallation » ci-dessous.

Règles : ne faites écouter le bridge que sur cette machine (`-addr` reste `127.0.0.1:7788` ; la page n’a pas d’authentification), ne le lancez pas avec `sudo`, et ne démarrez pas un second bridge à la même adresse.

#### Mise à jour

Téléchargez la nouvelle version comme à l’étape 2 et remplacez le programme comme à l’étape 3 ; le mod suit avec `claude plugin update symphony-bridge@symphony` (avec un clone comme marketplace, faites plutôt `git pull` dans ce clone). Le bridge en cours d’exécution garde l’ancien programme jusqu’à son redémarrage : `symphony -autostart off` puis `symphony -autostart on -open=false` (ou arrêtez-le et redémarrez-le comme il avait été démarré). Arrêter le bridge termine les sessions qu’il a démarrées : prévenez l’utilisateur avant. Le programme et le mod partagent le même numéro de version ; [CHANGELOG.md](CHANGELOG.md) liste ce qui a changé dans chaque version.

#### Désinstallation

```bash
symphony -autostart off                           # arrête le bridge et ses sessions, et ne le démarre plus à l’ouverture de session
claude plugin uninstall symphony-bridge@symphony  # le mod
claude plugin marketplace remove symphony         # l’entrée de la marketplace
rm ~/.local/bin/symphony                          # le programme, là où l’étape 3 l’a mis
```

Le dossier de données (les comptes avec leurs tokens, le journal des messages, les tâches) reste en place ; supprimez-le pour tout effacer.

## Fonctionnalités

- **Chaque session comme une discussion** : cliquez sur un employé et sa fenêtre s’ouvre au milieu de la page : en haut qui il est, en dessous la session sous forme de discussion : les instructions, la réflexion du modèle (repliée, « Réflexion… » en orange tant qu’elle dure), ses réponses au fil de l’eau, les appels d’outils, et les messages des autres sessions en violet. Écrivez en bas et le texte arrive à la session comme votre instruction ; le bouton Arrêter l’interrompt.
- **Approbations dans le navigateur** : les demandes d’autorisation (exécuter des commandes, modifier des fichiers) et les questions AskUserQuestion remontent sous forme de cartes par-dessus la zone de saisie de la fenêtre de cet employé, et le compteur rouge de la barre du haut vous y amène. Si le bridge ne tourne pas ou ne connaît pas le compte, la session revient au terminal comme d’habitude et ne reste jamais bloquée.
- **Un bureau en pixel art** : chaque session est un employé à un poste fixe. Vert : disponible ; jaune : occupé ; rouge : attend votre autorisation ou votre réponse ; gris : hors service. Les employés désœuvrés traînent d’abord à leur poste avec leur téléphone ou leurs jeux, puis partent se promener : télé sur le canapé de l’espace détente, baby-foot, partie de chat, discussion à la table ronde, café, coup d’œil par la fenêtre ; au bout d’un moment, ils font la sieste, souvent cachés dans la salle de réunion. La taille du personnage dépend du modèle, et ses muscles de l’effort de réflexion. Le bureau est un étage de l’immeuble d’une grande entreprise et remplit la fenêtre : l’open space au centre, de petites salles de réunion de 3 à 5 places avec leur plaque à gauche, la salle de réunion et l’espace détente à droite, et des ascenseurs pour arriver et repartir. La barre du haut règle aussi la taille des pixels, en quatre crans : le même bureau, dessiné avec des pixels plus fins et plus de détails à chaque cran.
- **Retrouver quelqu’un et le rappeler** : cliquez sur un employé disponible et il retourne à son poste (il y reste jusqu’à son prochain tour) ; le champ de recherche de la barre du haut trouve les employés par nom, par nom de session ou par dossier (touche `/` pour y aller), et un clic sur un compteur à côté (disponible, occupé, en attente, hors service) fait ressortir tous les employés dans cet état. L’onglet sous la barre la range vers le haut et la fait redescendre.
- **Messages entre sessions** : quand des sessions s’envoient des messages (SendMessage), une enveloppe traverse le bureau, et le journal des messages les conserve.
- **Démarrer des sessions depuis la page, sans qu’aucune fenêtre ne s’ouvre** : cliquez sur le « + » d’un poste libre pour embaucher un nouvel employé, puis choisissez le dossier, le modèle, l’effort de réflexion et le mode d’autorisation (un nouvel employé démarre avec tout autorisé, Opus et le maximum de réflexion) ; cliquez sur « Au travail ! » dans la fenêtre d’un employé hors service pour le faire revenir à son poste. La session tourne sur un terminal à elle, à l’intérieur du bridge : rien ne s’ouvre et rien ne vous prend le focus ; « Ouvrir le terminal » dans la fenêtre de l’employé montre ce terminal quand vous le voulez, et le fermer ne change rien.
- **Des équipes dans les salles de réunion** : cliquez sur une petite salle de réunion libre, écrivez une tâche et choisissez de 2 à 5 membres : des employés, qui reçoivent chacun une deuxième session rien que pour la tâche, ou des conseillers ponctuels. Par défaut, le premier membre dirige et distribue les parts avec SendMessage ; chaque membre travaille dans son propre worktree git, sur sa propre branche, si bien que personne ne marche sur les fichiers des autres. Les membres restent assis dans la salle jusqu’à la fin de la tâche.
- **10 langues** : English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) et Русский. La page suit la langue de votre navigateur et propose un sélecteur de langue dans la barre du haut ; les messages du bridge suivent `-lang`, sinon la langue de votre système (`LANG` et consorts), sinon l’anglais ; ceux du mod suivent `SYMPHONY_LANG` (la langue de la page, pour une session démarrée depuis la page), sinon la langue de votre système, sinon l’anglais.
- **Tout en local** : le bridge est un seul programme Go (bibliothèque standard uniquement ; les tests de bout en bout utilisent aussi chromedp), les pages sont en JavaScript pur, et rien ne dépend d’un service tiers.

![Approbations dans le navigateur](docs/approve.png)

## Prérequis

- Claude Code 2.1.316 (testé avec cette version). Le mod utilise les function hooks de Claude Code, une API encore en accès anticipé : après chaque mise à jour de Claude Code, relancez les tests de bout en bout.
- Le bridge existe en programme prêt à l’emploi pour Linux, macOS et Windows ; Go 1.27 ou plus récent n’est nécessaire que pour le compiler depuis les sources.
- `git`, pour les tâches en équipe seulement : chaque membre d’une tâche travaille dans un worktree git.
- Rien d’autre : les sessions démarrées depuis la page tournent dans le bridge, sur toutes les plateformes.

## Lancer le bridge

`symphony` démarre le bridge et ouvre le bureau dans votre navigateur à l’adresse <http://127.0.0.1:7788/> (`-open=false` le désactive) ; <http://127.0.0.1:7788/approve> est une page plus simple, réservée aux approbations. Relancer le bridge alors qu’il tourne déjà ouvre simplement le bureau. Sous Windows, lancé d’un double-clic, `symphony.exe` tourne dans sa propre fenêtre, et fermer cette fenêtre arrête le bridge.

| Option | Par défaut | Rôle |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Adresse d’écoute. La page n’a pas encore d’authentification : n’écoutez que sur cette machine |
| `-data` | `$SYMPHONY_DATA`, sinon `symphony` dans votre dossier de configuration | Le dossier de données : les comptes avec leurs tokens (`accounts.json`), le journal des messages (`messages.json`), les tâches (`tasks.json`) et le journal de `-background` (`bridge.log`) |
| `-claude` | `claude` | L’exécutable claude |
| `-lang` | la langue de votre système, sinon `en` | Langue des messages du bridge (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` ou `LANG` fixent la valeur par défaut) |
| `-mod` | vide | Pour le développement : donne aux sessions le mod de ce dossier avec `--plugin-dir`, à la place de la copie installée |
| `-web` | vide | Pour le développement : sert les pages depuis ce dossier, pour voir les modifications après un rechargement |
| `-open` | `true` | Ouvre le bureau dans le navigateur une fois le bridge démarré ; si un bridge tourne déjà sur `-addr`, ouvre simplement son bureau |
| `-autostart` | — | `on` : démarre le bridge à chaque ouverture de session, avec les autres options données, et le démarre tout de suite ; `off` : annule cela |
| `-background` | — | Démarre le bridge en arrière-plan, détaché du terminal et journalisant dans `bridge.log` du dossier de données, puis rend la main dès qu’il répond |
| `-version` | — | Affiche la version et quitte |

**Le lancer à l’ouverture de session** : `symphony -autostart on` fait démarrer le bridge à chaque ouverture de session, avec les options données en même temps (par exemple `-addr`), et le démarre tout de suite ; `symphony -autostart off` annule cela. Sous Linux, c’est un service utilisateur systemd (`journalctl --user -u symphony` affiche son journal), sous macOS un agent launchd (journal dans `~/Library/Logs/symphony.log`), sous Windows une valeur sous la clé Run du registre (le bridge tourne dans une fenêtre de console ; la fermer l’arrête jusqu’à la prochaine ouverture de session). Il faut le programme symphony lui-même, pas `go run`.

**Depuis les sources**, avec Go 1.27 ou plus récent : `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Démarrer des sessions

**Depuis la page** : cliquez sur le « + » d’un poste libre, saisissez un nom, puis choisissez le dossier et les options. Le bridge lance `claude` sur un terminal à lui, à l’intérieur du bridge : aucune fenêtre ne s’ouvre. La fenêtre de l’employé montre la session sous forme de discussion, et son bouton « Ouvrir le terminal » ouvre une fenêtre de terminal reliée à la session (`symphony attach <id>` fait de même depuis n’importe quel terminal ; `Ctrl-]` ou la fermeture de la fenêtre détache, et la session continue). Les sessions sont des processus enfants du bridge : arrêter le bridge les termine.

**Depuis un terminal** : créez un compte pour obtenir un token, puis lancez `claude` avec ce token :

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # la réponse contient le token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

Le mode d’autorisation `manual` est recommandé : en mode `auto`, la plupart des actions ne demandent rien à personne, donc peu d’approbations arrivent jusqu’à la page. Si le bridge n’est pas à l’adresse par défaut, définissez aussi `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` choisit la langue des messages du mod (sinon la langue de votre système, sinon l’anglais).

Quand rien ne répond à l’adresse du bridge sur cette machine, la session démarre elle-même le bridge avec `symphony -background -open=false -addr <that address>` (si `symphony` est dans votre PATH), et le bridge continue de tourner après la fin de la session. Définissez `SYMPHONY_DATA` si vos comptes ne sont pas dans le dossier de données par défaut.

## Tâches en équipe

Une petite salle de réunion, c’est l’endroit où une équipe de 2 à 5 sessions travaille ensemble sur une même tâche. Les tâches demandent `git` et le mod en version 0.7.0 (`claude plugin update symphony-bridge@symphony`).

**Lancer une tâche** : cliquez sur une petite salle de réunion libre et décrivez ce qu’il faut faire. Cochez les membres dans l’ordre voulu (de 2 jusqu’au nombre de places de la salle), choisissez le modèle et l’effort de réflexion de chacun ainsi que le mode d’autorisation de l’équipe (tout autorisé par défaut), puis cliquez sur « Lancer la tâche ». Un membre peut être :

- **Un employé** : il reçoit une deuxième session rien que pour la tâche, qui travaille dans le dossier de l’employé ; sa session habituelle, à son poste, reste telle quelle. Un employé n’est que dans une tâche à la fois, et celui qui n’a jamais travaillé n’a pas encore de dossier : cliquez d’abord une fois sur « Au travail ! ».
- **Un conseiller** : un participant ponctuel créé pour la tâche avec les champs habituels (nom, dossier, modèle, effort de réflexion). Il n’a pas de poste, arrive par l’ascenseur et disparaît à la fin de la tâche.

**Le chef** : par défaut, le premier membre dirige : il découpe la tâche, distribue les parts avec SendMessage, relit le travail, fusionne les branches des autres dans la sienne et vous dit ce qui a été fait. Décochez la case du chef et chacun reçoit toute la tâche, que les membres se répartissent entre eux. Dans les deux cas, le bridge donne à chaque membre sa première instruction : qui fait partie de l’équipe (les membres se joignent par leur nom avec SendMessage), la branche de chacun et les règles.

**Côté git** : chaque membre travaille dans son propre worktree git, sur sa propre branche `symphony/<task id>/<member>` de son dossier (par exemple `symphony/T1/bob` ; un second membre du même nom reçoit `-2`), si bien que personne ne marche sur les fichiers des autres et que votre propre copie de travail reste telle quelle. Les worktrees sont dans `.symphony/worktrees/` à l’intérieur du dossier, cachés à git par `.git/info/exclude` (votre `.gitignore` n’est pas touché). À la fin de la tâche, ce qu’un membre n’a pas encore commité est commité sur sa branche, donc rien ne se perd, et les worktrees sont supprimés.

- Dans vos propres dépôts, rien n’est fusionné : les branches restent, à vous de les fusionner ; s’il y a un chef, sa branche rassemble le travail de tous.
- Un dossier qui n’est pas un dépôt git en devient un pour la tâche (`git init` et un commit de tout ce qu’il contient) et le reste. À la fin de la tâche, les branches des membres y sont fusionnées, si bien que le dossier contient le résultat ; le `.git` et les branches sont conservés.
- Une fusion qui échoue est annulée et la base reste telle qu’elle était ; les branches restent, avec le travail qu’elles portent, et la fenêtre de la tâche indique quelle branche est en conflit.

**Dans le bureau** : les membres restent assis à la table de la salle pendant toute la tâche, et pendant ce temps le poste d’un employé montre une chaise vide. La plaque de la salle affiche l’identifiant de la tâche ; son voyant, vert tant que la salle est libre, passe au jaune pendant la tâche et au rouge quand un membre attend votre autorisation ou votre réponse. Cliquez sur la salle pour ouvrir la fenêtre de la tâche : la tâche, les membres avec leurs dossiers et leurs branches, la discussion de l’équipe et « Terminer la tâche ». Un clic sur le personnage d’un membre dans la salle ouvre aussi la fenêtre de la tâche ; un clic sur un membre dans cette fenêtre ouvre sa fenêtre de discussion juste à côté, la même que celle d’un employé, un membre à la fois (cliquez sur un autre membre pour changer). Le compteur rouge de la barre du haut et le champ de recherche ouvrent un membre d’une tâche de la même façon.

**La discussion de l’équipe** : la fenêtre de la tâche liste les messages que les membres s’envoient, chacun avec le personnage en pixels et le nom de son expéditeur, et le texte commence par `@` et son destinataire. En dessous, une zone de saisie vous permet d’écrire vous-même à l’équipe : `@bob-T1 …` ne va qu’à ce membre (plusieurs `@names` vont à chacun d’eux ; taper `@` ouvre la liste des membres, où l’on choisit avec les flèches, Entrée ou un clic), et un message sans `@` va à tous ceux de la tâche. Entrée envoie, Maj+Entrée saute une ligne.

Une tâche se termine quand vous la terminez depuis la fenêtre de la tâche ou quand sa dernière session prend fin : le bridge arrête les sessions des membres, s’occupe de la partie git comme décrit plus haut et supprime les conseillers. Une tâche encore en cours quand le bridge s’arrête est terminée de la même façon au prochain démarrage du bridge.

## Comptes et postes

- Dans symphony, une identité, c’est un nom, un token et un poste fixe. Un compte ne peut servir qu’à une session à la fois, plus une dans la tâche dont il fait partie ; toute autre session est refusée et ses approbations restent dans le terminal.
- La connexion à Claude n’est pas gérée ici : toutes les sessions peuvent partager le même identifiant Claude.
- Les données sont stockées dans `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) ou `%AppData%\symphony` (Windows), ou à l’endroit indiqué par `-data` / `SYMPHONY_DATA`. `accounts.json` contient les tokens et n’est lisible que par son propriétaire (0600) ; sous Windows, ce sont les droits d’accès du dossier utilisateur qui le protègent.

## Sécurité

- Le bridge n’accepte que les requêtes adressées à cette machine, et refuse les requêtes d’écriture qu’un autre site enverrait via votre navigateur : la page peut démarrer des sessions, c’est-à-dire exécuter des commandes sur cette machine.
- La page n’a pas encore d’authentification : **ne faites pas** écouter le bridge sur une autre adresse que celle de cette machine.
- Les programmes ne sont pas signés. macOS peut refuser une copie téléchargée avec un navigateur, faute de pouvoir vérifier le développeur : cliquez sur « Ouvrir quand même » dans Réglages Système → Confidentialité et sécurité, ou lancez une fois `xattr -d com.apple.quarantine symphony`. Sous Windows, SmartScreen peut aussi vous avertir : cliquez sur « Informations complémentaires », puis sur « Exécuter quand même ». Le fichier `checksums.txt` de la version donne le SHA-256 de chaque archive.

## Plateformes

| Système | Sessions démarrées depuis la page | « Ouvrir le terminal » | Tests |
| --- | --- | --- | --- |
| Linux | Un pseudo-terminal dans le bridge | Un terminal graphique (x-terminal-emulator, gnome-terminal, konsole, xterm) qui lance `symphony attach` | Tests de bout en bout complets |
| macOS | Un pseudo-terminal dans le bridge | Terminal.app lançant `symphony attach` | Tests de bout en bout complets |
| Windows | Une pseudo-console (ConPTY) dans le bridge | Une nouvelle fenêtre de console qui lance `symphony attach` | Compilation croisée et tests unitaires uniquement |

Le bridge répond « faire confiance à ce dossier » à votre place, sur toutes les plateformes.

## Développement

```bash
go test -race ./...                         # tests unitaires du bridge et des traductions
claude plugin test mods/symphony-bridge     # tests unitaires du mod
claude plugin validate .                    # vérifie la marketplace
claude plugin validate mods/symphony-bridge  # vérifie le mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # bout en bout : démarre de vraies sessions et consomme des tokens
```

- Travailler sur le mod : `go run ./cmd/bridge -mod mods/symphony-bridge`, et les sessions démarrées depuis la page reçoivent votre copie du mod via `--plugin-dir` ; pour les sessions que vous lancez à la main, ajoutez vous-même `--plugin-dir mods/symphony-bridge`.
- Travailler sur les pages : `go run ./cmd/bridge -web web`, et un rechargement affiche vos modifications.
- Voir le bureau sans vraies sessions : démarrez un second bridge et remplissez-le d’activité fictive : `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo`, puis `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Traductions : chaque langue tient dans un fichier, `web/locales/<lang>.json` pour les pages et `internal/i18n/locales/<lang>.json` pour le bridge, avec les mêmes clés que `en.json` ; les quelques messages du mod se trouvent dans `mods/symphony-bridge/hooks/messages.ts`. Les tests unitaires vérifient que chaque langue a toutes les clés et les mêmes `{placeholders}`, et le test de bout en bout `TestI18n` ouvre les pages dans chaque langue.
- Les tests de bout en bout ont besoin de claude, tmux (pour les sessions que les tests pilotent eux-mêmes) et Chromium ou Chrome : ils tournent donc sous Linux et macOS. Par défaut, ils utilisent le mod de ce dépôt ; définissez `SYMPHONY_E2E_MOD=installed` pour utiliser le mod installé. Les captures d’écran sont enregistrées dans `e2e/artifacts/`.
- Versions : augmentez la version dans `mods/symphony-bridge/.claude-plugin/plugin.json`, ajoutez une section `## vX.Y.Z (date)` à `CHANGELOG.md`, amenez dev sur main, puis étiquetez ce commit sur main et poussez l’étiquette (`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`) ; GitHub Actions vérifie que l’étiquette est la version de `plugin.json` et que la section existe, compile le programme pour toutes les plateformes et publie une version dont les notes commencent par cette section (`.github/workflows/release.yml` ; un suffixe comme `-rc.1` en fait une préversion). `.github/release.sh v0.7.0 dist` produit les mêmes archives en local.

| Dossier | Contenu |
| --- | --- |
| `mods/symphony-bridge/` | Le mod Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | La marketplace de plugins que lit `claude plugin install` |
| `cmd/bridge/`, `internal/bridge/` | Le bridge |
| `internal/i18n/` | La détection de la langue et les traductions du bridge |
| `web/` | Les pages du bureau et des approbations, avec leurs traductions, intégrées au bridge |
| `cmd/demo/` | De l’activité fictive pour le bureau |
| `e2e/` | Les tests de bout en bout |

## Licence

[MIT](LICENSE)
