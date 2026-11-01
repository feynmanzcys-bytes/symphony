# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | **Português (Brasil)** | [Русский](README.ru.md)

Coloque uma equipe de sessões do Claude Code num escritório em pixel art: veja de relance quem está trabalhando e quem está esperando por você, converse com cada um na própria janela dele, e resolva ali pedidos de permissão e perguntas do AskUserQuestion com um clique.

![O escritório](docs/office.png)

## Instalação

O jeito mais rápido: abra o Claude Code e cole

> Instale https://github.com/feynmanzcys-bytes/symphony para mim

O Claude Code lê este README e faz tudo o que está na próxima seção. Para instalar à mão, siga os mesmos passos.

### Para agentes de IA: o roteiro de instalação

Você está lendo isto porque um usuário pediu que você instalasse o symphony. Siga os passos abaixo em ordem, num shell, e termine com o relatório do passo 6. Não pergunte nada ao usuário, a não ser que um passo mande; tome você mesmo as decisões de rotina e liste-as no relatório.

O symphony tem duas partes e precisa das duas:

1. **O programa `symphony`** (o bridge): um único binário, um servidor web local em `http://127.0.0.1:7788/` que serve a página do escritório e roda as sessões. Ele precisa estar no `PATH` do usuário, com o nome `symphony`, porque o mod o inicia com `symphony -background` quando uma sessão não encontra nenhum bridge rodando.
2. **O plugin `symphony-bridge`** (o mod): um plugin do Claude Code do marketplace deste repositório, instalado no escopo de usuário. Ele só age nas sessões iniciadas com `SYMPHONY_TOKEN` e não faz nada em nenhuma outra sessão.

#### Passo 1: verificar a máquina

```bash
uname -sm                 # Linux/macOS: define o arquivo abaixo (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # precisa do Claude Code 2.1.x (testado com 2.1.316)
git --version             # só para as tarefas em equipe; se faltar, anote no relatório e não pare
symphony -version         # já instalado? mostra "symphony v0.7.1 (darwin/arm64)"
curl -s http://127.0.0.1:7788/api/state | head -c 40   # já há um bridge rodando? mostra {"type":"snapshot",...
claude plugin list --json # o mod já está instalado? procure "id":"symphony-bridge@symphony"
```

No Windows, use o PowerShell: `$env:PROCESSOR_ARCHITECTURE` é `AMD64` ou `ARM64`, e `Invoke-WebRequest http://127.0.0.1:7788/api/state` no lugar do curl.

Se o `claude` não existir, pare e diga ao usuário para instalar o Claude Code primeiro. Se o programa e o mod já estiverem no lugar, vá para "Atualizar", mais abaixo.

#### Passo 2: obter o programa

Use a primeira opção que funcionar:

- **A. Com `gh`** (funciona enquanto este repositório for privado, desde que `gh auth status` mostre uma sessão logada):
  ```bash
  gh release download -R feynmanzcys-bytes/symphony --pattern 'symphony_<os>_<arch>.tar.gz' -D /tmp/symphony-dl
  ```
- **B. Com `curl`** (só depois que o repositório for público; um 404 significa usar A ou C):
  ```bash
  curl -fL -o /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
  ```
- **C. Pelo código-fonte** (Go 1.27 ou mais recente; o clone é reaproveitado no passo 4):
  ```bash
  git clone https://github.com/feynmanzcys-bytes/symphony.git /tmp/symphony-src
  cd /tmp/symphony-src && go build -o symphony ./cmd/bridge
  ```
  Compile o binário; `go run` não serve, porque o `-autostart` recusa um build feito com `go run`.

| Sistema | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Cada arquivo traz o programa (`symphony`, ou `symphony.exe` no Windows), `LICENSE` e `README.md`. O `checksums.txt` da versão traz o SHA-256 de cada arquivo (`sha256sum -c --ignore-missing checksums.txt`, ou `shasum -a 256 -c` no macOS).

#### Passo 3: colocar no PATH

Escolha uma pasta que já esteja no `PATH` do usuário e que dê para gravar sem `sudo`: `~/.local/bin`, `/usr/local/bin` ou `/opt/homebrew/bin`, nessa ordem. Se nenhuma existir, crie `~/.local/bin`, adicione-a ao arquivo rc do shell (`~/.zshrc` ou `~/.bashrc`) e diga isso no relatório.

```bash
tar xzf /tmp/symphony-dl/symphony_<os>_<arch>.tar.gz -C /tmp/symphony-dl
install -m 755 /tmp/symphony-dl/symphony ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: o binário não é assinado
symphony -version          # mostra "symphony v0.7.1 (linux/amd64)": a versão e a plataforma
```

No Windows: `Expand-Archive symphony_windows_<arch>.zip -DestinationPath "$env:LOCALAPPDATA\Programs\symphony"`, depois adicione essa pasta ao `Path` do usuário (`[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','User'));$env:LOCALAPPDATA\Programs\symphony", 'User')`) e abra um shell novo. O SmartScreen pode avisar na primeira execução: "Mais informações" e depois "Executar assim mesmo".

Se o seu próprio shell não encontrar o `symphony` depois disso (o `PATH` dele foi lido antes da sua alteração), use o caminho absoluto no resto dos passos.

#### Passo 4: instalar o mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Isso adiciona o repositório como o marketplace de plugins `symphony` e instala o mod no escopo de usuário, sem perguntas. Ele clona o repositório com o git, então, enquanto o repositório for privado, precisa de acesso git ao GitHub; se falhar por isso, use um clone como marketplace (o do passo 2 C, ou faça um): `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. Com um clone como marketplace, as sessões leem o mod dessa pasta, e um `git pull` nela o atualiza.

Verifique:

```bash
claude plugin list --json   # uma entrada tem "id":"symphony-bridge@symphony", "scope":"user", "enabled":true
```

- `"scope":"project"` ou `"local"`: o mod só seria carregado numa pasta. Rode `claude plugin uninstall symphony-bridge@symphony` e instale de novo com `--scope user`.
- `"enabled":false`: `claude plugin enable symphony-bridge@symphony`.

#### Passo 5: iniciar o bridge

```bash
symphony -autostart on -open=false
```

Isso inicia o bridge agora e sempre que o usuário entrar no sistema (um serviço de usuário do systemd no Linux, um agente do launchd no macOS, a chave Run do registro no Windows) e mostra

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/
```

Se o usuário disse que não quer que ele inicie ao entrar no sistema, rode `symphony -background -open=false`: isso inicia o bridge separado do terminal e mostra `the symphony bridge runs in the background at http://127.0.0.1:7788/; its log: <data folder>/bridge.log`. Os dois comandos respondem `a symphony bridge is already running at http://127.0.0.1:7788/` quando já há um rodando; tudo bem, não inicie outro.

Verifique a cadeia inteira com uma única requisição:

```bash
curl -s http://127.0.0.1:7788/api/launch/options
```

A resposta traz `"enabled":true` e `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"0.7.1"}`: o bridge está rodando, encontrou o `claude`, e o mod está instalado onde as sessões vão carregá-lo. Com `"enabled":false`, `mod.code` diz o que está errado e `mod.fix`, o comando que resolve: `mod_not_installed` ou `mod_project_scope` (volte ao passo 4), `mod_disabled` (ative-o), `mod_check_failed` (`claude plugin list --json` falhou, então o Claude Code provavelmente é antigo demais). `"mod":null` significa que o bridge não encontrou o `claude` no `PATH` dele (o log dele diz isso): coloque-o lá, ou inicie o bridge com `-claude /path/to/claude`.

O log do bridge é o `bridge.log` na pasta de dados (`~/.config/symphony` no Linux, `~/Library/Application Support/symphony` no macOS, `%AppData%\symphony` no Windows); quando ele é iniciado ao entrar no sistema, o log vai para `journalctl --user -u symphony` no Linux e para `~/Library/Logs/symphony.log` no macOS.

#### Passo 6: relatório para o usuário

Abra o escritório no navegador (`open http://127.0.0.1:7788/` no macOS, `xdg-open` no Linux, `start` no Windows) e diga ao usuário:

- O escritório fica em `http://127.0.0.1:7788/`. Clique no "+" de uma mesa vazia para contratar o primeiro funcionário: escolha uma pasta, e uma sessão do Claude Code começa ali, dentro do bridge; a janela dela na página é o chat dela.
- Só as sessões iniciadas pela página (ou à mão com `SYMPHONY_TOKEN`, veja "Iniciar sessões" abaixo) aparecem no escritório. As outras sessões do Claude Code do usuário, incluindo a que está fazendo esta instalação, não aparecem.
- O que foi instalado e onde: o caminho do programa, o mod no escopo de usuário, o bridge iniciado ao entrar no sistema ou em segundo plano, qualquer alteração no `PATH` e, se for o caso, que o `git` está faltando.
- Como desfazer tudo: "Desinstalar", abaixo.

Regras: não deixe o bridge escutando em nada além desta máquina (`-addr` continua `127.0.0.1:7788`; a página não tem login), não o rode com `sudo` e não inicie um segundo bridge no mesmo endereço.

#### Atualizar

Baixe a nova versão como no passo 2 e substitua o programa como no passo 3; o mod acompanha com `claude plugin update symphony-bridge@symphony` (com um clone como marketplace, rode `git pull` nele). O bridge que está rodando continua com o programa antigo até reiniciar: `symphony -autostart off` e depois `symphony -autostart on -open=false` (ou pare e inicie do mesmo jeito que ele foi iniciado). Parar o bridge encerra as sessões que ele iniciou, então avise o usuário antes. O programa e o mod usam o mesmo número de versão; o [CHANGELOG.md](CHANGELOG.md) lista o que mudou em cada versão.

#### Desinstalar

```bash
symphony -autostart off                           # para o bridge e as sessões dele, e não o inicia mais ao entrar no sistema
claude plugin uninstall symphony-bridge@symphony  # o mod
claude plugin marketplace remove symphony         # a entrada do marketplace
rm ~/.local/bin/symphony                          # o programa, onde quer que o passo 3 o tenha colocado
```

A pasta de dados (as contas com seus tokens, o histórico de mensagens, as tarefas) continua lá; apague-a para remover tudo.

## O que ele faz

- **Cada sessão como um chat**: clique num funcionário e a janela dele abre no meio da página: em cima quem ele é, embaixo a sessão como um chat: os prompts, o raciocínio do modelo (recolhido, "Pensando…" em laranja enquanto dura), as respostas conforme chegam, as chamadas de ferramentas e as mensagens de outras sessões em roxo. Escreva embaixo e o texto chega à sessão como seu prompt; o botão Parar interrompe.
- **Aprovações no navegador**: pedidos de permissão (rodar comandos, editar arquivos) e perguntas do AskUserQuestion sobem como cartões por cima da caixa de texto da janela daquele funcionário, e o contador vermelho na barra do topo leva você até eles. Quando o bridge não está rodando ou não reconhece a conta, a sessão volta a usar o terminal, como sempre, e nunca fica travada.
- **Um escritório em pixel art**: cada sessão é um funcionário numa mesa fixa. Verde: livre; amarelo: trabalhando; vermelho: esperando sua aprovação ou resposta; cinza: fora do expediente. Os funcionários livres primeiro ficam na mesa mexendo no celular ou jogando videogame, depois saem para passear: veem TV no sofá da área de descanso, jogam pebolim ou pega-pega, batem papo na mesa redonda, pegam um café, olham pela janela; depois de um tempo tiram um cochilo, quase sempre escondidos na sala de reunião. O tamanho do personagem acompanha o modelo, e os músculos acompanham o esforço de raciocínio. O escritório é um andar do prédio de uma grande empresa e ocupa a janela inteira: a área de trabalho no centro, pequenas salas de reunião para 3 a 5 pessoas com placa à esquerda, a sala de reunião e a área de descanso à direita, e elevadores por onde se chega e se sai. Na barra do topo você também escolhe o tamanho do pixel, em quatro níveis: o mesmo escritório, desenhado com pixels mais finos e mais detalhes a cada nível.
- **Ache qualquer um e chame de volta**: clique num funcionário livre e ele volta para a mesa (e fica lá até a próxima tarefa); a busca na barra do topo encontra funcionários pelo nome, pelo nome da sessão ou pela pasta (tecle `/` para ir até ela), e um clique num contador ao lado dela (livre, trabalhando, pendente, fora do expediente) destaca todos nesse estado. A aba embaixo da barra a recolhe para cima e a traz de volta.
- **Mensagens entre sessões**: quando as sessões trocam mensagens (SendMessage), um envelope atravessa o escritório voando, e um histórico de mensagens guarda tudo.
- **Inicie sessões pela página, sem que nada se abra**: clique no "+" de uma mesa vazia para contratar um novo funcionário e escolha a pasta, o modelo, o esforço de raciocínio e o modo de permissão (um funcionário novo começa com tudo permitido, Opus e o máximo de raciocínio); clique em "Começar a trabalhar" num funcionário fora do expediente para trazê-lo de volta à mesma mesa. A sessão roda num terminal próprio dentro do bridge, então nenhuma janela abre e nada rouba o foco; "Abrir terminal" na janela do funcionário mostra esse terminal quando você quiser, e fechá-lo não muda nada.
- **Equipes nas salas de reunião**: clique numa sala de reunião pequena livre, escreva uma tarefa e escolha de 2 a 5 membros: funcionários, que ganham cada um uma segunda sessão só para a tarefa, ou consultores avulsos. Por padrão o primeiro membro lidera e distribui as partes com SendMessage; cada membro trabalha no próprio worktree do git, no próprio branch, então ninguém mexe nos arquivos de ninguém. Os membros ficam sentados na sala até a tarefa terminar.
- **10 idiomas**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) e Русский. A página segue o idioma do seu navegador e tem um seletor de idioma na barra do topo; as mensagens do bridge seguem o `-lang`, senão o idioma do seu sistema (`LANG` e afins), senão inglês; as do mod seguem o `SYMPHONY_LANG` (o idioma da página, numa sessão iniciada pela página), senão o idioma do seu sistema, senão inglês.
- **Tudo local**: o bridge é um único programa em Go (só com a biblioteca padrão; os testes de ponta a ponta também usam o chromedp), as páginas são JavaScript puro, e nada depende de serviços de terceiros.

![Aprovações no navegador](docs/approve.png)

## Requisitos

- Claude Code 2.1.316 (testado nesta versão). O mod usa os function hooks do Claude Code, que ainda são uma API em acesso antecipado; por isso, rode os testes de ponta a ponta depois de atualizar o Claude Code.
- O bridge vem como programa pronto para Linux, macOS e Windows; Go 1.27 ou mais recente só é necessário para compilá-lo a partir do código-fonte.
- `git`, só para as tarefas em equipe: cada membro de uma tarefa trabalha num worktree do git.
- Nada mais: as sessões iniciadas pela página rodam dentro do bridge em todas as plataformas.

## Rodar o bridge

`symphony` inicia o bridge e abre o escritório no seu navegador em <http://127.0.0.1:7788/> (`-open=false` desliga isso); <http://127.0.0.1:7788/approve> é uma página mais simples, só para aprovações. Iniciar o bridge de novo enquanto ele já está rodando só abre o escritório. No Windows, com um clique duplo, o `symphony.exe` roda numa janela própria, e fechar essa janela encerra o bridge.

| Opção | Padrão | O que faz |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Endereço de escuta. A página ainda não tem login, então use só endereços desta máquina |
| `-data` | `$SYMPHONY_DATA`; se não houver, `symphony` na sua pasta de configuração | A pasta de dados: as contas com seus tokens (`accounts.json`), o histórico de mensagens (`messages.json`), as tarefas (`tasks.json`) e o log do `-background` (`bridge.log`) |
| `-claude` | `claude` | O executável do claude |
| `-lang` | o idioma do seu sistema; senão, `en` | Idioma das mensagens do bridge (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` ou `LANG` definem o padrão) |
| `-mod` | vazio | Para desenvolvimento: passa às sessões o mod desta pasta com `--plugin-dir`, no lugar da cópia instalada |
| `-web` | vazio | Para desenvolvimento: serve as páginas desta pasta, e as alterações aparecem ao recarregar |
| `-open` | `true` | Abre o escritório no navegador quando o bridge sobe; se já houver um bridge em `-addr`, só abre o escritório dele |
| `-autostart` | — | `on`: inicia o bridge sempre que você entra no sistema, com as demais opções dadas, e já o inicia agora; `off`: desfaz isso |
| `-background` | — | Inicia o bridge em segundo plano, separado do terminal e com o log em `bridge.log` na pasta de dados, e volta quando ele responde |
| `-version` | — | Mostra a versão e sai |

**Iniciar ao entrar no sistema**: `symphony -autostart on` faz o bridge iniciar sempre que você entra no sistema, com as opções passadas junto (por exemplo `-addr`), e já o inicia agora; `symphony -autostart off` desfaz isso. No Linux é um serviço de usuário do systemd (`journalctl --user -u symphony` mostra o log), no macOS um agente do launchd (log em `~/Library/Logs/symphony.log`) e no Windows um valor na chave Run do registro (o bridge roda numa janela de console; fechá-la encerra o bridge até a próxima entrada no sistema). É preciso o próprio programa symphony, não o `go run`.

**Pelo código-fonte**, com Go 1.27 ou mais recente: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Iniciar sessões

**Pela página**: clique no "+" de uma mesa vazia, digite um nome e escolha a pasta e as opções. O bridge roda o `claude` num terminal próprio, dentro do bridge: nenhuma janela abre. A janela do funcionário mostra a sessão como um chat, e o botão "Abrir terminal" abre uma janela de terminal ligada à sessão (`symphony attach <id>` faz o mesmo de qualquer terminal; `Ctrl-]` ou fechar a janela desconecta, e a sessão continua). As sessões são processos filhos do bridge: parar o bridge encerra todas.

**Pelo terminal**: crie uma conta para obter um token e depois inicie o `claude` com ele:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # a resposta traz o token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

O modo de permissão recomendado é o `manual`: no modo `auto`, a maioria das ações não pergunta nada a ninguém, então poucas aprovações chegam à página. Se o bridge não estiver no endereço padrão, defina também `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` escolhe o idioma das mensagens do mod (senão, o idioma do seu sistema; senão, inglês).

Quando nada responde no endereço do bridge nesta máquina, a sessão inicia o bridge sozinha com `symphony -background -open=false -addr <that address>` (se o `symphony` estiver no seu PATH), e o bridge continua rodando depois que a sessão termina. Defina `SYMPHONY_DATA` se as suas contas não estiverem na pasta de dados padrão.

## Tarefas em equipe

Uma sala de reunião pequena é onde uma equipe de 2 a 5 sessões trabalha junta numa mesma tarefa. As tarefas precisam do `git` e do mod na versão 0.7.0 (`claude plugin update symphony-bridge@symphony`).

**Comece uma tarefa**: clique numa sala de reunião pequena livre e descreva o que precisa ser feito. Marque os membros na ordem que quiser (de 2 até os lugares da sala), escolha o modelo e o esforço de raciocínio de cada um e o modo de permissão da equipe (tudo permitido por padrão), e clique em "Começar a tarefa". Um membro pode ser:

- **Um funcionário**: ganha uma segunda sessão só para a tarefa, que trabalha na pasta do funcionário; a sessão dele na mesa fica como estava. Um funcionário participa de uma tarefa por vez, e quem nunca trabalhou ainda não tem pasta: clique antes uma vez em "Começar a trabalhar".
- **Um consultor**: um participante avulso criado para a tarefa com os campos de sempre (nome, pasta, modelo, esforço de raciocínio). Não tem mesa, chega pelo elevador e vai embora quando a tarefa termina.

**O líder**: por padrão o primeiro membro lidera: divide a tarefa, distribui as partes com SendMessage, revisa o trabalho, mescla os branches dos outros no seu e conta para você o que foi feito. Desmarque a caixa do líder e todos recebem a tarefa inteira e a dividem entre si. Nos dois casos, o bridge manda a cada membro o primeiro prompt: quem está na equipe (os membros se encontram pelo nome com SendMessage), o branch de cada um e as regras.

**Git**: cada membro trabalha no próprio worktree do git, no próprio branch `symphony/<task id>/<member>` (por exemplo `symphony/T1/bob`; um segundo membro com o mesmo nome ganha `-2`) da sua pasta, então ninguém mexe nos arquivos de ninguém e a sua cópia de trabalho fica como estava. Os worktrees ficam em `.symphony/worktrees/` dentro da pasta, escondidos do git pelo `.git/info/exclude` (o seu `.gitignore` não é tocado). Quando a tarefa termina, o que um membro não commitou é commitado no branch dele, então nada se perde, e os worktrees são removidos.

- Nos seus próprios repositórios nada é mesclado: os branches ficam para você mesclar; se houver líder, o branch dele reúne o trabalho de todos.
- Uma pasta que não é um repositório git vira um para a tarefa (`git init` e um commit de tudo o que há nela) e continua sendo. Quando a tarefa termina, os branches dos membros são mesclados nela, então a pasta fica com o resultado; o `.git` e os branches são mantidos.
- Uma mesclagem que falha é desfeita e a base fica como estava; os branches ficam, com o trabalho feito neles, e a janela da tarefa mostra qual branch deu conflito.

**No escritório**: os membros ficam sentados em volta da mesa da sala durante toda a tarefa, e enquanto isso a cadeira na mesa de trabalho do funcionário fica vazia. A placa da sala mostra o id da tarefa; a luz dela, verde enquanto a sala está livre, fica amarela enquanto a tarefa roda e vermelha enquanto um membro espera sua aprovação ou resposta. Clique na sala para abrir a janela da tarefa: a tarefa, os membros com suas pastas e branches, o chat da equipe e "Encerrar a tarefa". Um clique no boneco de um membro na sala também abre a janela da tarefa; um clique num membro na janela da tarefa abre a janela de chat dele encaixada ao lado, igual à janela de um funcionário, um membro de cada vez (clique em outro membro para trocar). O contador vermelho na barra do topo e a busca abrem um membro de uma tarefa do mesmo jeito.

**O chat da equipe**: a janela da tarefa lista as mensagens que os membros trocam entre si, cada uma com o boneco em pixel e o nome de quem mandou, e o texto começa com `@` e o destinatário. Embaixo fica uma caixa para você mesmo escrever para a equipe: `@bob-T1 …` vai só para esse membro (vários `@names` vão para todos eles; digitar `@` abre uma lista dos membros para escolher com as setas, Enter ou um clique), e uma mensagem sem `@` vai para todos da tarefa. Enter envia, Shift+Enter quebra a linha.

Uma tarefa termina quando você a encerra na janela da tarefa ou quando a última sessão dela termina: o bridge para as sessões dos membros, cuida da parte do git como descrito acima e apaga os consultores. Uma tarefa que ainda está rodando quando o bridge para é encerrada do mesmo jeito na próxima vez que o bridge iniciar.

## Contas e mesas

- No symphony, uma identidade é um nome, um token e uma mesa fixa. Uma conta atende uma sessão por vez, mais uma na tarefa de que participa; qualquer outra sessão é recusada, e as aprovações dela ficam no terminal.
- Os logins do Claude não são gerenciados aqui: todas as sessões podem compartilhar o mesmo login do Claude.
- Os dados ficam em `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) ou `%AppData%\symphony` (Windows), ou onde `-data` / `SYMPHONY_DATA` indicar. O `accounts.json` guarda os tokens e só o dono pode lê-lo (0600); no Windows, quem o protege são as permissões da pasta do usuário.

## Segurança

- O bridge só aceita requisições endereçadas a esta máquina e recusa requisições de escrita que outro site envie pelo seu navegador: a página pode iniciar sessões, o que significa executar comandos nesta máquina.
- A página ainda não tem login, então **não** deixe o bridge escutando em um endereço fora desta máquina.
- Os programas não são assinados. O macOS pode recusar uma cópia baixada pelo navegador por não conseguir verificar o desenvolvedor: clique em "Abrir Mesmo Assim" em Ajustes do Sistema → Privacidade e Segurança, ou rode uma vez `xattr -d com.apple.quarantine symphony`. O SmartScreen do Windows também pode avisar: clique em "Mais informações" e depois em "Executar assim mesmo". O `checksums.txt` da versão traz o SHA-256 de cada arquivo.

## Plataformas

| Sistema | Sessões iniciadas pela página | "Abrir terminal" | Testes |
| --- | --- | --- | --- |
| Linux | Um pseudoterminal dentro do bridge | Um terminal gráfico (x-terminal-emulator, gnome-terminal, konsole, xterm) rodando `symphony attach` | Testes de ponta a ponta completos |
| macOS | Um pseudoterminal dentro do bridge | Terminal.app rodando `symphony attach` | Testes de ponta a ponta completos |
| Windows | Um pseudoconsole (ConPTY) dentro do bridge | Uma nova janela de console rodando `symphony attach` | Só compilação cruzada e testes unitários |

O bridge confirma por você que a pasta é confiável ("trust this folder") em todas as plataformas.

## Desenvolvimento

```bash
go test -race ./...                         # testes unitários do bridge e das traduções
claude plugin test mods/symphony-bridge     # testes unitários do mod
claude plugin validate .                    # verifica o marketplace
claude plugin validate mods/symphony-bridge  # verifica o mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # ponta a ponta: inicia sessões de verdade e gasta tokens
```

- Para mexer no mod: `go run ./cmd/bridge -mod mods/symphony-bridge`, e as sessões iniciadas pela página recebem a sua cópia do mod com `--plugin-dir`; nas sessões iniciadas à mão, adicione você mesmo `--plugin-dir mods/symphony-bridge`.
- Para mexer nas páginas: `go run ./cmd/bridge -web web`, e basta recarregar para ver suas alterações.
- Para ver o escritório sem sessões de verdade: inicie um segundo bridge e alimente-o com atividade fictícia: `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo` e depois `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Traduções: cada idioma é um arquivo, `web/locales/<lang>.json` para as páginas e `internal/i18n/locales/<lang>.json` para o bridge, com as mesmas chaves do `en.json`; as poucas mensagens do mod ficam em `mods/symphony-bridge/hooks/messages.ts`. Os testes unitários verificam se cada idioma tem todas as chaves e os mesmos `{placeholders}`, e o teste de ponta a ponta `TestI18n` abre as páginas em todos os idiomas.
- Os testes de ponta a ponta precisam de claude, tmux (para as sessões que os próprios testes controlam) e Chromium ou Chrome, então rodam no Linux e no macOS. Por padrão, eles usam o mod deste repositório; defina `SYMPHONY_E2E_MOD=installed` para usar o mod instalado. As capturas de tela vão para `e2e/artifacts/`.
- Versões: aumente a versão em `mods/symphony-bridge/.claude-plugin/plugin.json`, adicione uma seção `## vX.Y.Z (date)` ao `CHANGELOG.md`, leve a dev para a main, depois marque esse commit da main com a tag e envie a tag (`git tag -a v0.7.0 -m v0.7.0 && git push origin v0.7.0`); o GitHub Actions verifica se a tag é a versão do `plugin.json` e se a seção existe, compila o programa para todas as plataformas e publica uma versão cujas notas começam com essa seção (`.github/workflows/release.yml`; um sufixo como `-rc.1` gera uma pré-versão). `.github/release.sh v0.7.0 dist` gera os mesmos arquivos localmente.

| Pasta | O que é |
| --- | --- |
| `mods/symphony-bridge/` | O mod do Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | O marketplace de plugins que o `claude plugin install` lê |
| `cmd/bridge/`, `internal/bridge/` | O bridge |
| `internal/i18n/` | Detecção de idioma e as traduções do bridge |
| `web/` | As páginas do escritório e de aprovações, com suas traduções, embutidas no bridge |
| `cmd/demo/` | Atividade fictícia para o escritório |
| `e2e/` | Testes de ponta a ponta |

## Licença

[MIT](LICENSE)
