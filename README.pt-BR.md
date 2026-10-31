# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | **Português (Brasil)** | [Русский](README.ru.md)

Coloque uma equipe de sessões do Claude Code num escritório em pixel art: veja de relance quem está trabalhando e quem está esperando por você, converse com cada um na própria janela dele, e resolva ali pedidos de permissão e perguntas do AskUserQuestion com um clique.

![O escritório](docs/office.png)

## O que ele faz

- **Cada sessão como um chat**: clique num funcionário e a janela dele abre no meio da página: em cima quem ele é, embaixo a sessão como um chat: os prompts, o raciocínio do modelo (recolhido, "Pensando…" em laranja enquanto dura), as respostas conforme chegam, as chamadas de ferramentas e as mensagens de outras sessões em roxo. Escreva embaixo e o texto chega à sessão como seu prompt; o botão Parar interrompe.
- **Aprovações no navegador**: pedidos de permissão (rodar comandos, editar arquivos) e perguntas do AskUserQuestion sobem como cartões por cima da caixa de texto da janela daquele funcionário, e o contador vermelho na barra do topo leva você até eles. Quando o bridge não está rodando ou não reconhece a conta, a sessão volta a usar o terminal, como sempre, e nunca fica travada.
- **Um escritório em pixel art**: cada sessão é um funcionário numa mesa fixa. Verde: livre; amarelo: trabalhando; vermelho: esperando sua aprovação ou resposta; cinza: fora do expediente. Os funcionários livres primeiro ficam na mesa mexendo no celular ou jogando videogame, depois saem para passear: veem TV no sofá da área de descanso, jogam pebolim ou pega-pega, batem papo na mesa redonda, pegam um café, olham pela janela; depois de um tempo tiram um cochilo, quase sempre escondidos na sala de reunião. O tamanho do personagem acompanha o modelo, e os músculos acompanham o esforço de raciocínio. O escritório é um andar do prédio de uma grande empresa e ocupa a janela inteira: a área de trabalho no centro, pequenas salas de reunião para 3 a 5 pessoas com placa à esquerda, a sala de reunião e a área de descanso à direita, e elevadores por onde se chega e se sai. Na barra do topo você também escolhe o tamanho do pixel, em quatro níveis: o mesmo escritório, desenhado com pixels mais finos e mais detalhes a cada nível.
- **Ache qualquer um e chame de volta**: clique num funcionário livre e ele volta para a mesa (e fica lá até a próxima tarefa); a busca na barra do topo encontra funcionários pelo nome ou pela pasta (tecle `/` para ir até ela), e um clique num contador ao lado dela (livre, trabalhando, pendente, fora do expediente) destaca todos nesse estado. A aba embaixo da barra a recolhe para cima e a traz de volta.
- **Mensagens entre sessões**: quando as sessões trocam mensagens (SendMessage), um envelope atravessa o escritório voando, e um histórico de mensagens guarda tudo.
- **Inicie sessões pela página, sem que nada se abra**: clique no "+" de uma mesa vazia para contratar um novo funcionário e escolha a pasta, o modelo, o esforço de raciocínio e o modo de permissão (um funcionário novo começa com tudo permitido, Opus e o máximo de raciocínio); clique em "Começar a trabalhar" na janela de um funcionário fora do expediente para trazê-lo de volta à mesma mesa. A sessão roda num terminal próprio dentro do bridge, então nenhuma janela abre e nada rouba o foco; "Abrir terminal" na janela do funcionário mostra esse terminal quando você quiser, e fechá-lo não muda nada.
- **Equipes nas salas de reunião**: clique numa sala de reunião pequena livre, escreva uma tarefa e escolha de 2 a 5 membros: funcionários, que ganham cada um uma segunda sessão só para a tarefa, ou consultores avulsos. Por padrão o primeiro membro lidera e distribui as partes com SendMessage; cada membro trabalha no próprio worktree do git, no próprio branch, então ninguém mexe nos arquivos de ninguém. Os membros ficam sentados na sala até a tarefa terminar.
- **10 idiomas**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) e Русский. A página segue o idioma do seu navegador e tem um seletor de idioma na barra do topo; as mensagens do bridge seguem o idioma do seu sistema (ou `-lang`), e as do mod seguem a página que iniciou a sessão.
- **Tudo local**: o bridge é um único programa em Go (só com a biblioteca padrão), as páginas são JavaScript puro, e nada depende de serviços de terceiros.

![Aprovações no navegador](docs/approve.png)

## Requisitos

- Claude Code 2.1.316 (testado nesta versão). O mod usa os function hooks do Claude Code, que ainda são uma API em acesso antecipado; por isso, rode os testes de ponta a ponta depois de atualizar o Claude Code.
- O bridge vem como programa pronto para Linux, macOS e Windows; Go 1.27 ou mais recente só é necessário para rodá-lo a partir do código-fonte.
- `git`, só para as tarefas em equipe: cada membro de uma tarefa trabalha num worktree do git.
- Nada mais: as sessões iniciadas pela página rodam dentro do bridge em todas as plataformas.

## Instalação

### 1. Instale o mod

No Claude Code, digite:

```
/plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Pressione `y` para adicionar o marketplace e depois escolha um escopo (o escopo de usuário, que é o padrão). Ou instale pelo shell:

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Se você clonou o repositório, pode usar o seu clone como marketplace. Assim, o mod e o bridge são o mesmo código, e depois de um `git pull` toda sessão nova já pega a versão nova:

```bash
claude plugin install symphony-bridge --marketplace /path/to/symphony
```

Instalado no escopo de usuário, o mod é carregado em toda sessão do Claude Code, mas só age nas sessões iniciadas com `SYMPHONY_TOKEN`; em todas as outras, ele não faz nada.

Para atualizar: `claude plugin update symphony-bridge@symphony`. Para desinstalar: `claude plugin uninstall symphony-bridge@symphony`.

### 2. Inicie o bridge

Baixe o arquivo do seu sistema na [versão mais recente](https://github.com/feynmanzcys-bytes/symphony/releases/latest), descompacte e rode o programa `symphony` que vem dentro:

| Sistema | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Por exemplo, no Linux:

```bash
curl -fLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz
tar xzf symphony_linux_amd64.tar.gz
./symphony
```

No Windows, descompacte o zip e rode `symphony.exe`; com um clique duplo, ele abre em uma janela própria, e fechar essa janela encerra o bridge.

Os programas não são assinados, então a primeira execução pode ser bloqueada. O macOS pode recusar uma cópia baixada pelo navegador por não conseguir verificar o desenvolvedor: clique em "Abrir Mesmo Assim" em Ajustes do Sistema → Privacidade e Segurança, ou rode uma vez `xattr -d com.apple.quarantine symphony`. No Windows, o SmartScreen também pode avisar: clique em "Mais informações" e depois em "Executar assim mesmo". O `checksums.txt` da versão traz o SHA-256 de cada arquivo.

O programa e o mod usam o mesmo número de versão: `symphony -version` mostra o do programa, e `claude plugin list`, o do mod. Para atualizar, baixe a nova versão e rode `claude plugin update symphony-bridge@symphony`. O [CHANGELOG.md](CHANGELOG.md) lista o que mudou em cada versão.

Ou rode o bridge a partir do código-fonte, com Go 1.27 ou mais recente:

```bash
git clone https://github.com/feynmanzcys-bytes/symphony.git
cd symphony
go run ./cmd/bridge
```

O escritório abre no seu navegador em <http://127.0.0.1:7788/> (`-open=false` desliga isso); <http://127.0.0.1:7788/approve> é uma página mais simples, só para aprovações. Iniciar o bridge de novo enquanto ele já está rodando só abre o escritório.

| Opção | Padrão | O que faz |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Endereço de escuta. A página ainda não tem login, então use só endereços desta máquina |
| `-data` | `$SYMPHONY_DATA`; se não houver, `symphony` na sua pasta de configuração | Onde ficam as contas (com seus tokens) e o histórico de mensagens |
| `-claude` | `claude` | O executável do claude |
| `-lang` | o idioma do seu sistema | Idioma das mensagens do bridge |
| `-mod` | vazio | Para desenvolvimento: passa às sessões o mod desta pasta com `--plugin-dir`, no lugar da cópia instalada |
| `-web` | vazio | Para desenvolvimento: serve as páginas desta pasta, e as alterações aparecem ao recarregar |
| `-open` | `true` | Abre o escritório no navegador quando o bridge sobe; se já houver um bridge em `-addr`, só abre o escritório dele |
| `-autostart` | — | `on`: inicia o bridge sempre que você entra no sistema, com as demais opções dadas, e já o inicia agora; `off`: desfaz isso |
| `-background` | — | Inicia o bridge em segundo plano, separado do terminal e com o log em `bridge.log` na pasta de dados, e volta quando ele responde |
| `-version` | — | Mostra a versão e sai |

**Iniciar ao entrar no sistema**: `symphony -autostart on` faz o bridge iniciar sempre que você entra no sistema, com as opções passadas junto (por exemplo `-addr`), e já o inicia agora; `symphony -autostart off` desfaz isso. No Linux é um serviço de usuário do systemd (`journalctl --user -u symphony` mostra o log), no macOS um agente do launchd (log em `~/Library/Logs/symphony.log`) e no Windows um valor na chave Run do registro (o bridge roda numa janela de console; fechá-la encerra o bridge até a próxima entrada no sistema). É preciso o próprio programa symphony, não o `go run`.

### 3. Inicie sessões

**Pela página**: clique no "+" de uma mesa vazia, digite um nome e escolha a pasta e as opções. O bridge roda o `claude` num terminal próprio, dentro do bridge: nenhuma janela abre. A janela do funcionário mostra a sessão como um chat, e o botão "Abrir terminal" abre uma janela de terminal ligada à sessão (`symphony attach <id>` faz o mesmo de qualquer terminal; `Ctrl-]` ou fechar a janela desconecta, e a sessão continua). As sessões são processos filhos do bridge: parar o bridge encerra todas.

**Pelo terminal**: crie uma conta para obter um token e depois inicie o `claude` com ele:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # a resposta traz o token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

O modo de permissão recomendado é o `manual`: no modo `auto`, a maioria das ações não pergunta nada a ninguém, então poucas aprovações chegam à página. Se o bridge não estiver no endereço padrão, defina também `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` escolhe o idioma das mensagens do mod.

Quando nada responde no endereço do bridge nesta máquina, a sessão inicia o bridge sozinha com `symphony -background` (se o `symphony` estiver no seu PATH), e o bridge continua rodando depois que a sessão termina. Defina `SYMPHONY_DATA` se as suas contas não estiverem na pasta de dados padrão.

## Tarefas em equipe

Uma sala de reunião pequena é onde uma equipe de 2 a 5 sessões trabalha junta numa mesma tarefa. As tarefas precisam do `git` e do mod na versão 0.7.0 (`claude plugin update symphony-bridge@symphony`).

**Comece uma tarefa**: clique numa sala de reunião pequena livre e descreva o que precisa ser feito. Marque os membros na ordem que quiser (de 2 até os lugares da sala), escolha o modelo e o esforço de raciocínio de cada um e o modo de permissão da equipe (tudo permitido por padrão), e clique em "Começar a tarefa". Há dois tipos de membro:

- **Funcionários**: cada um ganha uma segunda sessão só para a tarefa, que trabalha na pasta do funcionário; a sessão de sempre, a da mesa dele, fica como estava. Um funcionário só participa de uma tarefa por vez, e quem nunca trabalhou ainda não tem pasta: clique antes uma vez em "Começar a trabalhar".
- **Consultores**: participantes avulsos criados para a tarefa com os campos de sempre (nome, pasta, modelo, esforço de raciocínio). Não têm mesa, sobem pelo elevador e vão embora quando a tarefa termina.

**O líder**: por padrão o primeiro membro lidera: divide a tarefa, distribui as partes com SendMessage, revisa o trabalho, mescla os branches dos outros no seu e conta para você o que foi feito. Desmarque a caixa do líder e todos recebem a tarefa inteira e a dividem entre si. Nos dois casos, o bridge manda a cada membro o primeiro prompt: quem está na equipe (os membros se encontram pelo nome com SendMessage), o branch de cada um e as regras.

**O lado do git**: cada membro trabalha no próprio worktree do git, no próprio branch `symphony/<tarefa>/<nome>` da sua pasta, então ninguém mexe nos arquivos de ninguém e a sua cópia de trabalho fica como estava. Os worktrees ficam em `.symphony/worktrees/` dentro da pasta, escondidos do git pelo `.git/info/exclude` (o seu `.gitignore` não é tocado). Quando a tarefa termina, o que um membro não commitou é commitado no branch dele, então nada se perde, e os worktrees são removidos.

- Nos seus próprios repositórios nada é mesclado: os branches ficam para você mesclar; se houver líder, o branch dele reúne o trabalho de todos.
- Uma pasta que não é um repositório git vira um para a tarefa (`git init` e um commit de tudo o que há nela) e continua sendo. Quando a tarefa termina, os branches dos membros são mesclados nela, então a pasta fica com o resultado; o `.git` e os branches são mantidos.
- Uma mesclagem que falha é desfeita e a pasta fica como estava antes; os branches ficam, com o trabalho feito neles, e a janela da tarefa mostra qual branch deu conflito.

**No escritório**: os membros ficam sentados em volta da mesa da sala durante toda a tarefa, e enquanto isso a cadeira na mesa de trabalho do funcionário fica vazia. A placa da sala mostra o id da tarefa; a luz dela, verde enquanto a sala está livre, fica amarela enquanto a tarefa roda e vermelha enquanto um membro espera sua aprovação ou resposta. Clique na sala para abrir a janela da tarefa: a tarefa, os membros com suas pastas e branches, o chat da equipe e "Encerrar a tarefa". Um clique no boneco de um membro na sala também abre a janela da tarefa; um clique num membro ali abre a janela de chat dele encaixada ao lado, igual à janela de um funcionário, um membro de cada vez (clique em outro membro para trocar). O contador vermelho na barra do topo e a busca abrem um membro de uma tarefa do mesmo jeito.

**O chat da equipe**: a janela da tarefa lista as mensagens que os membros trocam entre si, cada uma com o boneco em pixel e o nome de quem mandou, e o texto começa com `@` e o destinatário. Embaixo fica uma caixa para você mesmo escrever para a equipe: `@bob-T1 …` vai só para esse membro (vários `@nomes` vão para todos eles; digitar `@` abre uma lista dos membros para escolher com as setas, Enter ou um clique), e uma mensagem sem `@` vai para todos da tarefa. Enter envia, Shift+Enter quebra a linha.

Uma tarefa termina quando você a encerra na janela da tarefa ou quando a última sessão dela termina: o bridge para as sessões dos membros, cuida da parte do git como descrito acima e apaga os consultores. Uma tarefa que ainda está rodando quando o bridge para é encerrada do mesmo jeito na próxima vez que o bridge iniciar.

## Contas e mesas

- No symphony, uma identidade é um nome, um token e uma mesa fixa. Uma conta atende uma sessão por vez, mais uma na tarefa de que participa; qualquer outra sessão é recusada, e as aprovações dela ficam no terminal.
- Os logins do Claude não são gerenciados aqui: todas as sessões podem compartilhar o mesmo login do Claude.
- Os dados ficam em `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) ou `%AppData%\symphony` (Windows). O `accounts.json` guarda os tokens e só o dono pode lê-lo (0600); no Windows, quem o protege são as permissões da pasta do usuário.

## Segurança

- O bridge só aceita requisições endereçadas a esta máquina e recusa requisições de escrita que outro site envie pelo seu navegador: a página pode iniciar sessões, o que significa executar comandos nesta máquina.
- A página ainda não tem login, então **não** deixe o bridge escutando em um endereço fora desta máquina.

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
claude plugin validate .                    # verifica o marketplace e o mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # ponta a ponta: inicia sessões de verdade e gasta tokens
```

- Para mexer no mod: `go run ./cmd/bridge -mod mods/symphony-bridge`, e as sessões iniciadas pela página recebem a sua cópia do mod com `--plugin-dir`; nas sessões iniciadas à mão, adicione você mesmo `--plugin-dir mods/symphony-bridge`.
- Para mexer nas páginas: `go run ./cmd/bridge -web web`, e basta recarregar para ver suas alterações.
- Para ver o escritório sem sessões de verdade: inicie um segundo bridge e alimente-o com atividade fictícia: `go run ./cmd/bridge -addr 127.0.0.1:7799 -data /tmp/demo` e depois `go run ./cmd/demo -base http://127.0.0.1:7799`.
- Traduções: cada idioma é um arquivo, `web/locales/<lang>.json` para as páginas e `internal/i18n/locales/<lang>.json` para o bridge, com as mesmas chaves do `en.json`; as poucas mensagens do mod ficam em `mods/symphony-bridge/hooks/messages.ts`. Os testes unitários verificam se cada idioma tem todas as chaves e os mesmos `{placeholders}`, e o teste de ponta a ponta `TestI18n` abre as páginas em todos os idiomas.
- Os testes de ponta a ponta precisam de claude, tmux (para as sessões que os próprios testes controlam) e Chromium ou Chrome, então rodam no Linux e no macOS. Por padrão, eles usam o mod deste repositório; defina `SYMPHONY_E2E_MOD=installed` para usar o mod instalado. As capturas de tela vão para `e2e/artifacts/`.
- Versões: marque um commit da main com a versão dele e envie a tag (`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`); o GitHub Actions então compila o programa para todas as plataformas e publica uma versão (`.github/workflows/release.yml`). A tag precisa ser a versão do mod em `plugin.json`. `.github/release.sh v0.3.0 dist` gera os mesmos arquivos localmente. Cada versão precisa de uma seção no `CHANGELOG.md`, que abre as notas dela.

| Pasta | O que é |
| --- | --- |
| `mods/symphony-bridge/` | O mod do Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | O marketplace de plugins que o `/plugin install` lê |
| `cmd/bridge/`, `internal/bridge/` | O bridge |
| `internal/i18n/` | Detecção de idioma e as traduções do bridge |
| `web/` | As páginas do escritório e de aprovações, com suas traduções, embutidas no bridge |
| `cmd/demo/` | Atividade fictícia para o escritório |
| `e2e/` | Testes de ponta a ponta |

## Licença

[MIT](LICENSE)
