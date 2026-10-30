# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | **Português (Brasil)** | [Русский](README.ru.md)

Coloque uma equipe de sessões do Claude Code num escritório em pixel art: veja de relance quem está trabalhando e quem está esperando por você, e resolva pedidos de permissão e perguntas do AskUserQuestion com um clique, num pop-up na página web.

![O escritório](docs/office.png)

## O que ele faz

- **Aprovações no navegador**: pedidos de permissão (rodar comandos, editar arquivos) e perguntas do AskUserQuestion vão para a página web. Quando o bridge não está rodando ou não reconhece a conta, a sessão volta a usar o terminal, como sempre, e nunca fica travada.
- **Um escritório em pixel art**: cada sessão é um funcionário numa mesa fixa. Verde: livre; amarelo: trabalhando; vermelho: esperando sua aprovação ou resposta; cinza: fora do expediente. Os funcionários livres primeiro ficam na mesa mexendo no celular ou jogando videogame, depois saem para passear: veem TV no sofá da área de descanso, jogam pebolim ou pega-pega, batem papo na mesa redonda, pegam um café, olham pela janela; depois de um tempo tiram um cochilo, quase sempre escondidos na sala de reunião. O tamanho do personagem acompanha o modelo, e os músculos acompanham o esforço de raciocínio. O escritório é um andar do prédio de uma grande empresa e ocupa a janela inteira: a área de trabalho no centro, pequenas salas de reunião para 3 a 5 pessoas com placa à esquerda, a sala de reunião e a área de descanso à direita, e elevadores por onde se chega e se sai. Na barra do topo você também escolhe o tamanho do pixel, em quatro níveis: o mesmo escritório, desenhado com pixels mais finos e mais detalhes a cada nível.
- **Ache qualquer um e chame de volta**: clique num funcionário livre e ele volta para a mesa (e fica lá até a próxima tarefa); a busca na barra do topo encontra funcionários pelo nome ou pela pasta (tecle `/` para ir até ela), e um clique num contador ao lado dela (livre, trabalhando, pendente, fora do expediente) destaca todos nesse estado. A aba embaixo da barra a recolhe para cima e a traz de volta.
- **Mensagens entre sessões**: quando as sessões trocam mensagens (SendMessage), um envelope atravessa o escritório voando, e um histórico de mensagens guarda tudo.
- **Inicie sessões pela página**: clique no "+" de uma mesa vazia para contratar um novo funcionário e escolha a pasta, o modelo, o esforço de raciocínio e o modo de permissão; clique em "Começar a trabalhar" num funcionário fora do expediente para trazê-lo de volta à mesma mesa.
- **10 idiomas**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) e Русский. A página segue o idioma do seu navegador e tem um seletor de idioma na barra do topo; as mensagens do bridge seguem o idioma do seu sistema (ou `-lang`), e as do mod seguem a página que iniciou a sessão.
- **Tudo local**: o bridge é um único programa em Go (só com a biblioteca padrão), as páginas são JavaScript puro, e nada depende de serviços de terceiros.

![Aprovações no navegador](docs/approve.png)

## Requisitos

- Claude Code 2.1.314 (testado nesta versão). O mod usa os function hooks do Claude Code, que ainda são uma API em acesso antecipado; por isso, rode os testes de ponta a ponta depois de atualizar o Claude Code.
- O bridge vem como programa pronto para Linux, macOS e Windows; Go 1.27 ou mais recente só é necessário para rodá-lo a partir do código-fonte.
- Para iniciar sessões pela página: no Linux e no macOS, o ideal é usar o tmux.

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

O programa e o mod usam o mesmo número de versão: `symphony -version` mostra o do programa, e `claude plugin list`, o do mod. Para atualizar, baixe a nova versão e rode `claude plugin update symphony-bridge@symphony`.

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
| `-data` | `symphony` na sua pasta de configuração | Onde ficam as contas (com seus tokens) e o histórico de mensagens |
| `-terminal` | `auto` | Terminal das sessões iniciadas pela página: `auto`, `tmux` ou `window` |
| `-claude` | `claude` | O executável do claude |
| `-lang` | o idioma do seu sistema | Idioma das mensagens do bridge |
| `-mod` | vazio | Para desenvolvimento: passa às sessões o mod desta pasta com `--plugin-dir`, no lugar da cópia instalada |
| `-web` | vazio | Para desenvolvimento: serve as páginas desta pasta, e as alterações aparecem ao recarregar |
| `-open` | `true` | Abre o escritório no navegador quando o bridge sobe; se já houver um bridge em `-addr`, só abre o escritório dele |
| `-version` | — | Mostra a versão e sai |

### 3. Inicie sessões

**Pela página**: clique no "+" de uma mesa vazia, digite um nome e escolha a pasta e as opções. O bridge abre um terminal que roda o `claude`; com tmux, a sessão se chama `sym-<id>`, e `tmux attach -t sym-<id>` leva você até lá.

**Pelo terminal**: crie uma conta para obter um token e depois inicie o `claude` com ele:

```bash
curl -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # a resposta traz o token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

O modo de permissão recomendado é o `manual`: no modo `auto`, a maioria das ações não pergunta nada a ninguém, então poucas aprovações chegam à página. Se o bridge não estiver no endereço padrão, defina também `SYMPHONY_BRIDGE=http://host:port`. `SYMPHONY_LANG` escolhe o idioma das mensagens do mod.

## Contas e mesas

- No symphony, uma identidade é um nome, um token e uma mesa fixa. Uma conta atende uma sessão por vez; uma segunda sessão é recusada, e as aprovações dela ficam no terminal.
- Os logins do Claude não são gerenciados aqui: todas as sessões podem compartilhar o mesmo login do Claude.
- Os dados ficam em `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) ou `%AppData%\symphony` (Windows). O `accounts.json` guarda os tokens e só o dono pode lê-lo (0600); no Windows, quem o protege são as permissões da pasta do usuário.

## Segurança

- O bridge só aceita requisições endereçadas a esta máquina e recusa requisições de escrita que outro site envie pelo seu navegador: a página pode iniciar sessões, o que significa executar comandos nesta máquina.
- A página ainda não tem login, então **não** deixe o bridge escutando em um endereço fora desta máquina.

## Plataformas

| Sistema | Terminal das sessões iniciadas pela página | Testes |
| --- | --- | --- |
| Linux | tmux, se estiver instalado; senão, um terminal gráfico | Testes de ponta a ponta completos |
| macOS | Terminal.app por padrão; `-terminal tmux` para usar o tmux | Só compilação cruzada e testes unitários |
| Windows | Uma nova janela de console | Só compilação cruzada e testes unitários |

Só o tmux consegue confirmar por você que a pasta é confiável ("trust this folder"). Com uma janela de terminal, a primeira sessão numa pasta nova precisa de um clique seu nessa janela.

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
- Os testes de ponta a ponta precisam de claude, tmux e chromium, então rodam no Linux e no macOS com tmux. Por padrão, eles usam o mod deste repositório; defina `SYMPHONY_E2E_MOD=installed` para usar o mod instalado. As capturas de tela vão para `e2e/artifacts/`.
- Versões: marque um commit da main com a versão dele e envie a tag (`git tag -a v0.3.0 -m v0.3.0 && git push origin v0.3.0`); o GitHub Actions então compila o programa para todas as plataformas e publica uma versão (`.github/workflows/release.yml`). A tag precisa ser a versão do mod em `plugin.json`. `.github/release.sh v0.3.0 dist` gera os mesmos arquivos localmente.

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
