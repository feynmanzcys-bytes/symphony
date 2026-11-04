# symphony

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | **Português (Brasil)** | [Русский](README.ru.md)

O symphony é um projeto independente. Não é afiliado à Anthropic, nem endossado ou patrocinado por ela. Claude e Claude Code são marcas comerciais da Anthropic, PBC.

Coloque uma equipe de sessões do Claude Code numa só página, como colegas em um app de mensagens: veja de relance quem está trabalhando e quem está esperando por você, fale com cada um na própria conversa dele, e resolva ali pedidos de permissão e perguntas do AskUserQuestion com um clique.

![Colegas: as sessões listadas à esquerda com seus rostos, uma conversa no meio, o perfil e o computador à direita](docs/coworkers.png)

Esse é o **Colegas**, a página que o symphony abre: as sessões listadas à esquerda, os rostos mostrando o que fazem, a conversa no meio, o perfil e o computador à direita, e as equipes como chats em grupo. O mesmo escritório também vem como **o escritório em pixels**, um andar do prédio de uma empresa onde as sessões são funcionários nas suas mesas: escolha-o em Configurações → Tema, e o bridge lembra.

![O escritório em pixels: os funcionários nas mesas da área de trabalho, as pequenas salas de reunião à esquerda, a sala de reunião e a área de descanso à direita](docs/office.png)

## Instalação

O jeito mais rápido: abra o Claude Code e cole

> Instale https://github.com/feynmanzcys-bytes/symphony para mim

O Claude Code lê este README e faz os passos da próxima seção; ele pergunta a você antes de qualquer alteração que dure além da instalação. Para instalar à mão, siga os mesmos passos.

### Para agentes de IA: o roteiro de instalação

Você está lendo isto porque um usuário pediu que você instalasse o symphony. Siga os passos abaixo em ordem, num shell, e termine com o relatório do passo 6. Tome você mesmo as decisões de rotina (o arquivo, a pasta no `PATH`), mas pergunte ao usuário antes de qualquer alteração que permaneça depois desta instalação: iniciar o symphony a cada entrada no sistema, editar um arquivo rc do shell ou alterar o `PATH` do usuário. Liste todas as decisões no relatório.

O symphony tem duas partes e precisa das duas:

1. **O programa `symphony`** (o bridge): um único binário, um servidor web local em `http://127.0.0.1:7788/` que serve a página do escritório e roda as sessões. Ele deve estar no `PATH` do usuário, com o nome `symphony`, porque o mod o inicia com `symphony -background` quando uma sessão não encontra nenhum bridge rodando.
2. **O plugin `symphony-bridge`** (o mod): um plugin do Claude Code do marketplace deste repositório, instalado no escopo de usuário. Ele só age nas sessões iniciadas com `SYMPHONY_TOKEN` e não faz nada em nenhuma outra sessão.

#### Passo 1: verificar a máquina

```bash
uname -sm                 # Linux/macOS: define o arquivo abaixo (x86_64 → amd64; arm64/aarch64 → arm64)
claude --version          # precisa do Claude Code 2.1.x (testado com 2.1.317)
git --version             # só para as tarefas em equipe; se faltar, anote no relatório e não pare
symphony -version         # já instalado? mostra "symphony v1.6.0 (darwin/arm64)"
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:7788/api/state   # um bridge rodando? 401: symphony 1.x; 200: um mais antigo; 000: nenhum
claude plugin list --json # o mod já está instalado? procure uma entrada com "id": "symphony-bridge@symphony"
brew --version            # tem Homebrew (macOS, Linux)? o passo 2 pode instalar com ele
```

No Windows, use o PowerShell: `$env:PROCESSOR_ARCHITECTURE` é `AMD64` ou `ARM64`, e `scoop --version` responde quando o Scoop está instalado.

Se o `claude` não existir, pare e diga ao usuário para instalar o Claude Code primeiro. Se o programa e o mod já estiverem no lugar, vá para "Atualizar", mais abaixo. Se só um deles estiver, faça os passos do outro: os passos 2 e 3 para o programa (a não ser que `symphony -version` já mostre a versão mais recente), o passo 4 para o mod e depois o passo 5. Um bridge que já está rodando continua com o programa com que foi iniciado: se houver um, você vai pará-lo antes do passo 5.

#### Passo 2: obter o programa

**Com Homebrew ou Scoop.** Este repositório também é um tap do Homebrew e um bucket do Scoop. Se o passo 1 encontrou o Homebrew (macOS, Linux) ou o Scoop (Windows), você pode instalar com ele no lugar do resto deste passo e do passo 3: ele baixa o arquivo da versão mais recente, confere com o SHA-256 da fórmula ou do manifesto (escrito a partir do `checksums.txt` da versão quando ela saiu) e põe o `symphony` no `PATH`. Depois siga para o passo 4 e diga no relatório com qual dos dois instalou.

```bash
brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
brew install symphony
symphony -version          # mostra a versão e a plataforma
```

```powershell
scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony
scoop install symphony
symphony -version
```

Senão, baixe o arquivo para esta máquina e o `checksums.txt` da versão para uma pasta nova, só para eles, e confira o arquivo antes de instalar qualquer coisa. Os arquivos são `symphony_<os>_<arch>.tar.gz`, com `<os>` `linux` ou `darwin` (macOS) e `<arch>` `amd64` ou `arm64`, e `symphony_windows_<arch>.zip`.

No Linux e no macOS:

```bash
d=$(mktemp -d)   # uma pasta nova que só você pode ler
echo "$d"        # anote: o passo 3 instala a partir dela
cd "$d"
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_<os>_<arch>.tar.gz
curl -fsSLO https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/checksums.txt
if command -v sha256sum >/dev/null; then sha256sum -c --ignore-missing checksums.txt; else shasum -a 256 -c --ignore-missing checksums.txt; fi   # precisa mostrar "symphony_<os>_<arch>.tar.gz: OK"
tar xzf symphony_<os>_<arch>.tar.gz              # o programa agora é "$d/symphony"
```

Se a verificação não mostrar `OK`, pare: não instale o programa e avise o usuário. Com o `gh` no lugar do curl, baixe para `$d` (o gh cria a pasta quando ela ainda não existe) e depois rode ali a mesma verificação e o `tar`:

```bash
gh release download -R feynmanzcys-bytes/symphony -p 'symphony_<os>_<arch>.tar.gz' -p checksums.txt -D "$d"
```

No Windows, no PowerShell:

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$zip = "symphony_windows_$arch.zip"
$d = Join-Path $env:TEMP ("symphony-" + [guid]::NewGuid())   # uma pasta nova
New-Item -ItemType Directory -Path $d | Out-Null
$url = 'https://github.com/feynmanzcys-bytes/symphony/releases/latest/download'
Invoke-WebRequest -UseBasicParsing -Uri "$url/$zip" -OutFile (Join-Path $d $zip)
Invoke-WebRequest -UseBasicParsing -Uri "$url/checksums.txt" -OutFile (Join-Path $d 'checksums.txt')
$want = ((Select-String -SimpleMatch -Path (Join-Path $d 'checksums.txt') -Pattern $zip).Line -split '\s+')[0]
$got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $d $zip)).Hash
if ($got -ne $want) { throw "checksum mismatch: $zip" } else { 'checksum OK' }
```

Depois, se o `gh` estiver instalado e com login feito (`gh auth status` dá certo), confira onde o arquivo foi compilado: todo arquivo de uma versão traz o atestado do GitHub de que o workflow de publicação deste repositório o compilou. Fora de um terminal, o `gh` não mostra nada quando a verificação passa, então vá pelo código de saída:

```bash
gh attestation verify "$d/symphony_<os>_<arch>.tar.gz" --repo feynmanzcys-bytes/symphony && echo 'attestation OK'
```

```powershell
gh attestation verify (Join-Path $d $zip) --repo feynmanzcys-bytes/symphony
if ($LASTEXITCODE -eq 0) { 'attestation OK' }
```

Versões anteriores à v1.2.0 não têm atestado (`no attestations found`); para elas, vale a verificação do SHA-256. Qualquer outra falha: pare, como se a verificação do SHA-256 falhasse. Sem o `gh`, ou sem login, pule esta verificação e diga isso no relatório.

**Pelo código-fonte** no Linux ou no macOS, quando o usuário preferir (Go 1.27 ou mais recente). Compile o binário; `go run` não serve, porque o `-autostart` recusa um build feito com `go run`:

```bash
d=$(mktemp -d)
git clone https://github.com/feynmanzcys-bytes/symphony.git "$d/src"
cd "$d/src" && go build -o "$d/symphony" ./cmd/bridge   # o programa agora é "$d/symphony", como se viesse de um arquivo
```

| Sistema | x86-64 | ARM64 |
| --- | --- | --- |
| Linux | [`symphony_linux_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_amd64.tar.gz) | [`symphony_linux_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_linux_arm64.tar.gz) |
| macOS | [`symphony_darwin_amd64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_amd64.tar.gz) (Intel) | [`symphony_darwin_arm64.tar.gz`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_darwin_arm64.tar.gz) (Apple silicon) |
| Windows | [`symphony_windows_amd64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_amd64.zip) | [`symphony_windows_arm64.zip`](https://github.com/feynmanzcys-bytes/symphony/releases/latest/download/symphony_windows_arm64.zip) |

Cada arquivo traz o programa (`symphony`, ou `symphony.exe` no Windows), `LICENSE` e `README.md`. O `checksums.txt` da versão traz o SHA-256 de cada arquivo.

#### Passo 3: colocar no PATH

Instale o programa do passo 2, `$d/symphony` (se o seu shell não guarda variáveis de um comando para outro, escreva por extenso a pasta que o passo 2 mostrou). Use uma pasta que já esteja no `PATH` do usuário e que dê para gravar sem `sudo`: `~/.local/bin`, `/usr/local/bin` ou `/opt/homebrew/bin`, nessa ordem.

```bash
install -m 755 "$d/symphony" ~/.local/bin/symphony
xattr -d com.apple.quarantine ~/.local/bin/symphony 2>/dev/null || true   # macOS: o binário não é assinado
symphony -version          # mostra "symphony v1.6.0 (linux/amd64)": a versão e a plataforma
```

Se nenhuma dessas pastas estiver no `PATH`, pergunte ao usuário antes de alterar qualquer coisa. Se ele disser sim, crie `~/.local/bin`, adicione-a ao arquivo rc do shell (`~/.zshrc` ou `~/.bashrc`) e diga isso no relatório; se disser não, instale o programa onde ele indicar e avise que o mod não consegue iniciar o bridge sozinho enquanto o `symphony` não estiver no `PATH` das sessões do Claude Code dele (iniciar ao entrar no sistema, no passo 5, continua funcionando).

No Windows, descompacte-o numa pasta própria (`$d` e `$zip` vêm do passo 2; num shell novo, defina-os de novo):

```powershell
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Expand-Archive -LiteralPath (Join-Path $d $zip) -DestinationPath $dest -Force
& (Join-Path $dest 'symphony.exe') -version   # mostra "symphony v1.6.0 (windows/amd64)"
```

Adicionar essa pasta ao `Path` do usuário é uma alteração duradoura: pergunte ao usuário antes. Se ele disser sim, rode a linha abaixo e abra um shell novo; se disser não, use o caminho completo. Extraído com `Expand-Archive`, o programa não leva a marca do download e o SmartScreen o deixa rodar; uma cópia extraída com "Extrair tudo" do Explorador de Arquivos pode ser bloqueada ao abrir com clique duplo: "Mais informações" e depois "Executar assim mesmo".

```powershell
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dest", 'User')
```

Se o seu próprio shell não encontrar o `symphony` depois disso (o `PATH` dele foi lido antes da sua alteração), use o caminho absoluto no resto dos passos.

#### Passo 4: instalar o mod

```bash
claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
```

Isso adiciona o repositório como o marketplace de plugins `symphony` e instala o mod no escopo de usuário, sem perguntas. Um clone local do repositório também pode servir de marketplace: `claude plugin install symphony-bridge --marketplace /absolute/path/to/symphony`. As sessões passam então a ler o mod dessa pasta, e um `git pull` nela o atualiza; por isso, guarde um clone assim num lugar duradouro, não numa pasta temporária.

Verifique:

```bash
claude plugin list --json   # uma entrada tem "id": "symphony-bridge@symphony", "scope": "user", "enabled": true
```

- `"scope": "project"` ou `"local"`: o mod só seria carregado numa pasta. Rode `claude plugin uninstall symphony-bridge@symphony` e instale de novo com `--scope user`.
- `"enabled": false`: `claude plugin enable symphony-bridge@symphony`.

#### Passo 5: iniciar o bridge

Se o passo 1 encontrou um bridge rodando, pare-o primeiro, como em "Atualizar", mais abaixo: ele roda o programa antigo, e o novo só o encontraria ali e sairia do caminho. Pará-lo encerra as sessões que ele iniciou, então avise o usuário antes.

Pergunte ao usuário se o symphony deve iniciar sempre que ele entrar no sistema.

Se sim:

```bash
symphony -autostart on -open=false
```

Isso inicia o bridge agora e sempre que o usuário entrar no sistema (um serviço de usuário do systemd no Linux, um agente do launchd no macOS, a chave Run do registro no Windows) e mostra, depois da data e da hora:

```
symphony now starts when you log in (<where>); it's running at http://127.0.0.1:7788/?key=<key>
```

Se não:

```bash
symphony -background -open=false
```

Isso inicia o bridge separado do terminal (ele não inicia de novo depois que a máquina reinicia) e mostra, na saída de erro padrão, depois da data e da hora:

```
the symphony bridge runs in the background at http://127.0.0.1:7788/?key=<key>; its log: <data folder>/bridge.log
```

Quando já há um bridge desta versão rodando, ele mostra (do mesmo jeito) `a symphony bridge is already running at http://127.0.0.1:7788/?key=<key>` no lugar disso; tudo bem, não inicie outro. `<key>` é a chave das páginas, que dá a um navegador acesso ao escritório: não a cole em nenhum outro lugar. Se o programa responder `127.0.0.1:7788 is taken: another program, or a symphony bridge with another data folder, answers there`, diga ao usuário o que está respondendo ali (um bridge do symphony mais antigo também conta: pare-o como em "Atualizar") e não escolha outro endereço sem perguntar.

Verifique a cadeia inteira com uma única requisição, que apresenta a chave das páginas tirada da pasta de dados (pela entrada padrão, para que a chave não fique numa linha de comando que outros usuários poderiam ver):

```bash
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-${XDG_CONFIG_HOME:-$HOME/.config}/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options      # Linux
printf 'X-Symphony-Key: %s\n' "$(cat "${SYMPHONY_DATA:-$HOME/Library/Application Support/symphony}/key")" | curl -s -H @- http://127.0.0.1:7788/api/launch/options   # macOS
```

```powershell
$data = if ($env:SYMPHONY_DATA) { $env:SYMPHONY_DATA } else { Join-Path $env:APPDATA 'symphony' }
(Invoke-WebRequest -UseBasicParsing -Headers @{ 'X-Symphony-Key' = (Get-Content (Join-Path $data 'key')) } -Uri http://127.0.0.1:7788/api/launch/options).Content
```

A pasta de dados é `~/.config/symphony` no Linux (`$XDG_CONFIG_HOME/symphony` quando essa variável está definida), `~/Library/Application Support/symphony` no macOS e `%AppData%\symphony` no Windows, ou `SYMPHONY_DATA` quando essa variável está definida.

A resposta traz `"enabled":true` e `"mod":{"ok":true,"id":"symphony-bridge@symphony","version":"1.1.1"}`: o bridge está rodando, encontrou o `claude`, e o mod está instalado onde as sessões vão carregá-lo. Com `"enabled":false`, `mod.code` diz o que está errado e `mod.fix`, o comando que resolve: `mod_not_installed` ou `mod_project_scope` (volte ao passo 4), `mod_disabled` (ative-o), `mod_check_failed` (`claude plugin list --json` falhou, então o Claude Code provavelmente é antigo demais). `"mod":null` significa que o bridge não encontrou o `claude` no `PATH` dele (o log dele diz isso): coloque-o lá, ou inicie o bridge com `-claude /path/to/claude`. Uma resposta com `"code":"page_key"` significa que a requisição não apresentou chave nenhuma, ou apresentou a errada: confira se o `cat` encontrou o arquivo da chave (a pasta de dados, acima); se encontrou, o bridge que está rodando foi iniciado com outra pasta de dados.

O log do bridge é o `bridge.log` na pasta de dados; quando ele é iniciado ao entrar no sistema, o log vai para `journalctl --user -u symphony` no Linux e para `~/Library/Logs/symphony.log` no macOS.

#### Passo 6: relatório para o usuário

Abra o escritório rodando `symphony`: com o bridge rodando, ele abre o escritório no navegador padrão, já com acesso, e sai. Ele também mostra o endereço com `?key=`, que dá acesso a qualquer navegador desta máquina.

Diga ao usuário:

- Para abrir o escritório, rode `symphony` (ou dê um clique duplo nele); `http://127.0.0.1:7788/` sozinho funciona num navegador que já tem acesso. Clique em "+ Colega", embaixo da lista, para contratar o primeiro funcionário: dê um nome a ele, e uma sessão do Claude Code começa dentro do bridge, numa pasta nova só dele (ou na escolhida em "Configurações avançadas"); a conversa dela na página é o chat dela. Configurações → Colegas → "Todos os colegas" (Funcionários no escritório de pixels) lista todos os funcionários e importa uma lista de novos; Configurações → Tema troca para o escritório em pixels.
- Os funcionários novos seguem as configurações do Claude Code do usuário (modelo, esforço de raciocínio, modo de permissão), a não ser que ele escolha outras na caixa de diálogo. `bypassPermissions` executa todo comando e toda edição sem perguntar, e o que uma sessão lê pode então fazê-la executar comandos em nome do usuário; a caixa de diálogo avisa quando ele é escolhido.
- Cada sessão gasta o plano do Claude ou os créditos de API do usuário, como qualquer sessão do Claude Code; uma tarefa roda de 2 a 5 sessões ao mesmo tempo.
- Só as sessões iniciadas pela página (ou à mão com `SYMPHONY_TOKEN`, veja "Iniciar sessões" abaixo) aparecem no escritório. As outras sessões do Claude Code do usuário, incluindo a que está fazendo esta instalação, não aparecem.
- O que foi instalado e onde: o caminho do programa, o mod no escopo de usuário, o bridge iniciado ao entrar no sistema ou em segundo plano, qualquer alteração no `PATH` ou num arquivo rc, as decisões que você tomou e, se for o caso, que o `git` está faltando.
- Como desfazer tudo: "Desinstalar", abaixo.

Regras: o bridge só escuta nesta máquina (ele recusa qualquer endereço que não seja desta máquina; mantenha o padrão `127.0.0.1:7788`, a não ser que o usuário escolha outra porta), não o rode com `sudo` e não inicie um segundo bridge no mesmo endereço nem com a mesma pasta de dados (o symphony recusa e diz qual está rodando).

#### Atualizar

Baixe a nova versão como no passo 2. O bridge que está rodando continua com o programa antigo até reiniciar (e no Windows não dá para substituir um programa em execução), então pare-o com `symphony -stop`, substitua o programa como no passo 3 e inicie-o de novo do jeito que ele rodava antes: `symphony -autostart on -open=false` se ele inicia ao entrar no sistema (isso também já o inicia agora), senão `symphony -background -open=false`. Se o bridge roda com um `-addr` ou um `-data` diferente do padrão, passe os mesmos a cada um desses comandos: o `-stop` só para o bridge daquela pasta de dados (em `-addr`, ou onde quer que o bridge dessa pasta esteja rodando), e diz `nothing to stop` quando nenhum responde. Parar o bridge encerra as sessões que ele iniciou, então avise o usuário antes. (O `-stop` veio com a v1.1.0; para parar um bridge v1.0.0, encerre o processo dele: `pkill -x symphony` no Linux e no macOS, `Stop-Process -Name symphony` no Windows.)

Se instalou com Homebrew ou Scoop, `brew update && brew upgrade symphony` (ou `scoop update; scoop update symphony`) substitui o download e o passo 3, entre o mesmo `-stop` e o novo início. Se o symphony inicia sempre que o usuário entra no sistema, a entrada desse início aponta o link do Homebrew para o programa (`bin/symphony`), que a atualização muda para a nova versão; então aí o `-autostart on` só o inicia de novo agora.

O mod acompanha com `claude plugin update symphony-bridge@symphony` (com um clone como marketplace, rode `git pull` nele).

O programa e o mod usam o mesmo número de versão; o [CHANGELOG.md](CHANGELOG.md) lista o que mudou em cada versão.

#### Desinstalar

```bash
symphony -stop                                    # para o bridge e as sessões dele, não importa como ele foi iniciado (acrescente o -addr e o -data dele se não forem os padrões)
symphony -autostart off                           # não o inicia mais ao entrar no sistema (avisa quando ele já não iniciava)
claude plugin uninstall symphony-bridge@symphony  # o mod
claude plugin marketplace remove symphony         # a entrada do marketplace
rm ~/.local/bin/symphony                          # o programa, onde quer que o passo 3 o tenha colocado
```

Se o passo 3 adicionou `~/.local/bin` a um arquivo rc do shell, remova essa linha também. No Windows, no PowerShell:

```powershell
symphony -stop                                               # para o bridge e as sessões dele (acrescente o -addr e o -data dele se não forem os padrões)
symphony -autostart off                                      # não o inicia mais ao entrar no sistema
claude plugin uninstall symphony-bridge@symphony
claude plugin marketplace remove symphony
$dest = Join-Path $env:LOCALAPPDATA 'Programs\symphony'
Remove-Item -Recurse -Force $dest                            # o programa
$p = ([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -and $_ -ne $dest }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $p, 'User')    # a entrada do Path que o passo 3 adicionou
```

Se instalou com Homebrew, rode `brew uninstall symphony` e `brew untap feynmanzcys-bytes/symphony` no lugar da linha do `rm`; com Scoop, `scoop uninstall symphony` e `scoop bucket rm symphony` no lugar das quatro últimas linhas do PowerShell.

A pasta de dados (as contas com seus tokens, a chave das páginas, o histórico de mensagens, o registro de consumo, as tarefas, e arquivos como `lock`, `bridge.addr` ou um `*.corrupt-<time>` deixado de lado) continua lá; apague-a para remover tudo. As pastas próprias dos funcionários (`symphony-bots` na pasta pessoal, ou a pasta escolhida nas configurações) também continuam lá, com o trabalho deles. Apagá-la também apaga a chave das páginas: o próximo início cria uma nova, e todos os navegadores precisam obter acesso de novo. No Linux, com um navegador snap ou flatpak, o `open.html` fica na pasta própria desse navegador (`~/snap/<snap>/common` ou `~/.var/app/<app id>`); apague-o também.

## O que ele faz

- **Cada sessão como um chat**: clique num colega na lista e a conversa dele abre no meio da página: em cima quem ele é, embaixo a sessão como um chat: os prompts, o raciocínio do modelo (recolhido, "Pensando…" em laranja enquanto dura), as respostas conforme chegam, as chamadas de ferramentas e as mensagens de outras sessões em roxo. Escreva embaixo e o texto chega à sessão como seu prompt; o botão Parar interrompe. O que o modelo escreve aparece como Markdown (títulos, listas, tabelas, código, links); um bloco de código cercado é colorido para as linguagens comuns (Go, JavaScript/TypeScript, Python, Rust, C, Java, Shell, JSON, YAML, HTML, CSS, SQL, Diff …), pelo tokenizador da própria página. Entre duas partes de uma resposta, o raciocínio e as chamadas de ferramentas se recolhem em uma linha, “Pensando… 2 min 38 s” enquanto ele trabalha e “Pensou por 2 min 38 s” ao terminar; um clique expande as etapas.
- **Aprovações no navegador**: pedidos de permissão (rodar comandos, editar arquivos) e perguntas do AskUserQuestion aparecem como cartões na conversa daquele colega, no ponto em que surgiram, e você continua podendo digitar (no escritório em pixels, eles sobem por cima da caixa de texto da janela do funcionário); um colega que está esperando por você fica vermelho no topo da lista, e o título da aba conta os cartões. "Permitir nesta sessão" no cartão de uma permissão deixa passar o mesmo tipo de chamada sem perguntar até a sessão terminar: toda edição, um comando pelo jeito como ele começa (`npm test`) ou uma ferramenta. O formulário de um servidor MCP também aparece como cartão; você responde a ele no terminal da sessão, que o cartão abre ali mesmo na janela. Quando o bridge não está rodando, não reconhece a conta ou não é confiável, a sessão volta a usar o terminal, como sempre, e nunca fica travada.
- **O escritório em pixels, o outro tema**: cada sessão é um funcionário numa mesa fixa. Verde: livre; amarelo: trabalhando; vermelho: esperando sua aprovação ou resposta; cinza: fora do expediente. Os funcionários livres primeiro ficam na mesa mexendo no celular ou jogando videogame, depois saem para passear: veem TV no sofá da área de descanso, jogam pebolim ou pega-pega, batem papo na mesa redonda, pegam um café, olham pela janela; depois de um tempo tiram um cochilo, quase sempre escondidos na sala de reunião. O tamanho do personagem acompanha o modelo, e os músculos acompanham o esforço de raciocínio. O escritório é um andar do prédio de uma grande empresa e ocupa a janela inteira: a área de trabalho no centro, pequenas salas de reunião para 3 a 5 pessoas com placa à esquerda, a sala de reunião e a área de descanso à direita, e elevadores por onde se chega e se sai. Nas configurações você também escolhe o tamanho do pixel, em quatro níveis: o mesmo escritório, desenhado com pixels mais finos e mais detalhes a cada nível.
- **Ache qualquer um**: a busca acima da lista encontra colegas pelo nome, pelo nome da sessão ou pela pasta, e chats em grupo pela tarefa (tecle `/` para ir até ela). No escritório em pixels, clique num funcionário livre e ele volta para a mesa (e fica lá até a próxima tarefa); nesse tema, a busca fica na barra do topo, e um clique num contador ao lado dela (livre, trabalhando, pendente, fora do expediente) destaca todos nesse estado. A aba embaixo da barra a recolhe para cima e a traz de volta.
- **Mensagens entre sessões**: quando as sessões trocam mensagens (SendMessage), a mensagem aparece em roxo nas duas conversas; no escritório em pixels, um envelope também atravessa o escritório voando, e um histórico de mensagens guarda tudo.
- **Inicie sessões pela página, sem que nada se abra**: clique em "+ Colega" (no escritório em pixels, no "+" de uma mesa vazia) para contratar um novo funcionário: dê um nome a ele e inicie a sessão. Onde e como ele trabalha ficam recolhidos em "Configurações avançadas", numa linha que diz o que ele vai usar, conforme Configurações → Funcionários diz que um funcionário novo começa: uma pasta nova só dele dentro da pasta padrão (`symphony-bots` na sua pasta pessoal), iniciada como repositório git, à qual ele volta da próxima vez; o modelo, o esforço de raciocínio e o modo de permissão, que seguem as suas configurações do Claude Code ("Padrão"), a não ser que você escolha outros; e se é para dizer à sessão quem mais está no escritório (o prompt de sistema dela cita então as outras sessões online, uma vez por conversa, para que ela possa falar com elas pelo SendMessage). Expanda a linha para mudar qualquer uma dessas escolhas só para este início, ou para escolher uma pasta. O modo de permissão `bypassPermissions` mostra um aviso em vermelho do que ele permite, com as configurações avançadas recolhidas ou não. Clique em "Começar a trabalhar" na conversa de um funcionário fora do expediente para trazê-lo de volta (à mesma mesa, no escritório em pixels), com as escolhas da última vez em que trabalhou. A sessão roda num terminal próprio dentro do bridge, então nenhuma janela abre e nada rouba o foco; o computador ao lado da conversa mostra esse terminal ao vivo, "Assumir" o abre numa janela própria (no escritório em pixels, "Abrir terminal" na janela do funcionário), e fechar essa janela não muda nada.
- **Todos os funcionários numa lista**: Configurações → Colegas → "Todos os colegas" (Funcionários no escritório de pixels) lista cada funcionário com o estado, a pasta e aquilo com que ele começa: o modelo, o esforço de raciocínio, o modo de permissão e o contexto do escritório (quem nunca foi iniciado pela página segue as configurações até ser). Edite um na linha dele, ou selecione vários e mude-os juntos, ou exclua os selecionados que estão fora do expediente. "Importar…" recebe uma lista, colada ou de um arquivo (um nome por linha, ou o JSON que "Exportar" gera), diz o que faria, entrada por entrada, e então cria os funcionários novos e muda os outros de uma vez, ou não faz nada enquanto houver uma entrada errada. Uma mudança vale a partir do próximo início do funcionário; os nomes ficam como estão.
- **Equipes como chats em grupo**: clique em "+ Grupo" (no escritório em pixels, numa sala de reunião pequena livre), escreva uma tarefa e escolha de 2 a 5 membros: funcionários, que ganham cada um uma segunda sessão só para a tarefa, ou consultores avulsos. Por padrão o primeiro membro lidera e distribui as partes com SendMessage; cada membro trabalha no próprio worktree do git, no próprio branch, então ninguém mexe nos arquivos de ninguém. No escritório em pixels, os membros ficam sentados na sala até a tarefa terminar.
- **Configurações e relatar um problema**: a engrenagem acima da lista (Configurações na barra do escritório em pixels) reúne o que você muda de vez em quando: o idioma, o tema, claro ou escuro, o tamanho do pixel, pausar o movimento, as etapas expandidas e aquilo com que um funcionário novo começa (a pasta, o modelo, o esforço de raciocínio, o modo de permissão e o contexto do escritório; os consultores e o modo de permissão de uma equipe nova também partem dali). O que dá errado (um início ou uma tarefa que falhou, um erro de API de uma sessão, um erro na página) é escrito sozinho em um registro de erros; quando quiser relatar um problema, clique lá em “Quero relatar um problema” e envie para nós o arquivo que ele dá, por e-mail ou anexado a uma issue no GitHub. O arquivo traz os erros, as versões e as últimas linhas do registro do bridge, mas nenhuma chave, token ou conversa.
- **10 idiomas**: English, 简体中文, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Português (Brasil) e Русский. A página segue o idioma do seu navegador e tem um seletor de idioma nas configurações; as mensagens do bridge seguem o `-lang`, senão o idioma do seu sistema (`LANG` e afins), senão inglês; as do mod seguem o `SYMPHONY_LANG` (o idioma da página, numa sessão iniciada pela página), senão o idioma do seu sistema, senão inglês.
- **Tudo local**: o bridge é um único programa em Go (só com a biblioteca padrão; os testes de ponta a ponta também usam o chromedp) que só escuta nesta máquina, as páginas são JavaScript puro, e nada depende de serviços de terceiros.

![Uma conversa em Colegas: a sessão como um chat, com um cartão de pergunta e um cartão de permissão no ponto em que surgiram](docs/approve.png)

![Configurações → Todos os colegas: cada funcionário com a pasta e aquilo com que começa, dois deles selecionados para mudar juntos](docs/staff.png)

## Requisitos

- Claude Code 2.1.317 (testado nesta versão). O mod usa os function hooks do Claude Code, que ainda são uma API em acesso antecipado; por isso, rode os testes de ponta a ponta depois de atualizar o Claude Code.
- O bridge vem como programa pronto para Linux, macOS e Windows; Go 1.27 ou mais recente só é necessário para compilá-lo a partir do código-fonte.
- `git`, só para as tarefas em equipe: cada membro de uma tarefa trabalha num worktree do git.
- Nada mais: as sessões iniciadas pela página rodam dentro do bridge em todas as plataformas.

## Rodar o bridge

`symphony` inicia o bridge e abre o escritório no seu navegador, já com acesso (`-open=false` desliga isso). O navegador recebe a chave das páginas do bridge como cookie por meio do `open.html`, um arquivo na pasta de dados que só você pode ler, então a chave nunca aparece numa linha de comando; no Linux, quando o seu navegador é um snap ou um flatpak, esse arquivo vai para a pasta própria do navegador, porque esses navegadores não conseguem ler a pasta de dados. Abrir <http://127.0.0.1:7788/> num navegador sem acesso mostra uma página que explica como entrar: rodar o `symphony` de novo, ou abrir o endereço com `?key=` que o bridge mostrou no log dele. <http://127.0.0.1:7788/approve> é uma página mais simples, só para aprovações. Rodar o `symphony` de novo enquanto ele já está rodando só abre o escritório. No Windows, com um clique duplo, o `symphony.exe` roda numa janela própria, e fechar essa janela encerra o bridge.

| Opção | Padrão | O que faz |
| --- | --- | --- |
| `-addr` | `127.0.0.1:7788` | Endereço de escuta. O host tem de ser `127.0.0.1`, `localhost` ou `::1` (como em `[::1]:7788`); o bridge recusa qualquer outro |
| `-data` | `$SYMPHONY_DATA`; se não houver, `symphony` na sua pasta de configuração | A pasta de dados: as contas com seus tokens (`accounts.json`), a chave das páginas (`key`) e o `open.html`, o histórico de mensagens (`messages.json`), o registro de consumo (`ledger.json`), as tarefas (`tasks.json`), as configurações do bridge (`settings.json`), o registro de erros (`errors.json`), o log do `-background` (`bridge.log`), `lock` e `bridge.addr` (qual bridge a usa, e onde), e um histórico de mensagens, registro de consumo ou configurações danificados, deixados de lado como `*.corrupt-<time>` |
| `-claude` | `claude` | O executável do claude |
| `-lang` | o idioma do seu sistema; senão, `en` | Idioma das mensagens do bridge (`SYMPHONY_LANG`, `LC_ALL`, `LC_MESSAGES` ou `LANG` definem o padrão) |
| `-mod` | vazio | Para desenvolvimento: passa às sessões o mod desta pasta com `--plugin-dir`, no lugar da cópia instalada |
| `-web` | vazio | Para desenvolvimento: serve as páginas desta pasta, e as alterações aparecem ao recarregar |
| `-open` | `true` | Abre o escritório no navegador quando o bridge sobe; se o bridge desta pasta de dados já estiver rodando em `-addr`, só abre o escritório dele |
| `-autostart` | — | `on`: inicia o bridge sempre que você entra no sistema, com as demais opções dadas, e já o inicia agora; `off`: desfaz isso |
| `-background` | — | Inicia o bridge em segundo plano, separado do terminal e com o log em `bridge.log` na pasta de dados, e volta quando ele responde |
| `-stop` | — | Para o bridge desta pasta de dados (em `-addr`, senão onde quer que ele esteja rodando), não importa como ele foi iniciado (as sessões dele se encerram), e volta quando ele já tiver parado; ele só apresenta a chave a um bridge que prove que a tem |
| `-version` | — | Mostra a versão e sai |

Quando outra coisa responde em `-addr` (outro programa, um bridge com outra pasta de dados ou um bridge do symphony mais antigo), o symphony avisa e para; ele só apresenta a chave das páginas a um bridge que prove que a tem. Uma pasta de dados é de um bridge por vez: iniciado com uma pasta de dados cujo bridge já roda em outro endereço, o symphony diz onde ele roda, abre o escritório dele e para (dê a um segundo bridge o próprio `-data`).

**Iniciar ao entrar no sistema**: `symphony -autostart on` faz o bridge iniciar sempre que você entra no sistema, com as opções passadas junto (por exemplo `-addr`), e já o inicia agora; `symphony -autostart off` desfaz isso. No Linux é um serviço de usuário do systemd (`journalctl --user -u symphony` mostra o log), no macOS um agente do launchd (log em `~/Library/Logs/symphony.log`) e no Windows um valor na chave Run do registro (o bridge roda numa janela de console; fechá-la encerra o bridge até a próxima entrada no sistema). `symphony -stop` para um bridge não importa como ele foi iniciado, até que ele seja iniciado de novo (na próxima entrada no sistema, no caso de um iniciado ao entrar no sistema). É preciso o próprio programa symphony, não o `go run`. O bridge iniciado ao entrar no sistema não recebe nada do seu shell além do `PATH`, então o `-autostart on` também guarda as variáveis de que as sessões dele podem precisar, do jeito que o seu shell as tem quando você roda o comando: `CLAUDE_CONFIG_DIR`, o proxy (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, `ALL_PROXY`, em maiúsculas ou minúsculas), `NODE_EXTRA_CA_CERTS`, `SSL_CERT_FILE` e `SSL_CERT_DIR`, e nada mais (nenhuma chave de API), em `autostart-env.json` na pasta de dados, que só você pode ler; rode o comando de novo depois de mudá-las. Um bridge que falha ao iniciar (a pasta de dados dele em uso por outro bridge, um `accounts.json` danificado, outra coisa no endereço dele) é reiniciado algumas vezes e depois deixado em paz: 5 tentativas em um minuto no Linux (`systemctl --user status symphony` diz o porquê), uma tentativa por minuto no macOS (o log diz o porquê); corrija a causa e rode `symphony -autostart on` de novo, ou simplesmente entre no sistema de novo.

**Pelo código-fonte**, com Go 1.27 ou mais recente: `git clone https://github.com/feynmanzcys-bytes/symphony.git && cd symphony && go run ./cmd/bridge`.

## Iniciar sessões

**Pela página**: clique em "+ Colega" (o "+" de uma mesa vazia no escritório em pixels), digite um nome e, se quiser, escolha a pasta e as opções. O bridge roda o `claude` num terminal próprio, dentro do bridge: nenhuma janela abre. A conversa do funcionário mostra a sessão como um chat, e "Assumir", embaixo do computador dele ("Abrir terminal" na janela do funcionário no escritório em pixels), abre uma janela de terminal ligada à sessão (`symphony attach <id>` faz o mesmo de qualquer terminal, com `-addr` e `-data` quando o bridge não usa os padrões; quando não é possível abrir uma janela de terminal, a página mostra o comando exato. `Ctrl-]` ou fechar a janela desconecta, e a sessão continua). As sessões são processos filhos do bridge: parar o bridge encerra todas. Quando uma sessão termina (parada pela página, junto com o bridge ou saindo sozinha), o que ela deixou rodando em segundo plano (um comando que uma ferramenta pôs em segundo plano com `&`, por exemplo) termina junto: no Windows, o job object da sessão encerra tudo; no Linux e no macOS, o bridge encontra esses processos pelo id de início no ambiente deles, que no macOS ele não consegue ler nos programas da própria Apple (`/bin/sleep`, `bash`, `zsh`, `perl`: o sistema esconde o ambiente deles do `ps -E`), então um servidor `node` ou do Homebrew deixado para trás termina, e um simples `sleep` ou script de shell, não.

**Pelo terminal**: crie uma conta para obter um token, apresentando a chave das páginas, e depois inicie o `claude` com o token:

```bash
printf 'X-Symphony-Key: %s\n' "$(cat ~/.config/symphony/key)" | curl -H @- -X POST 127.0.0.1:7788/api/accounts -d '{"name":"frontend-alice"}'   # a resposta traz o token
SYMPHONY_TOKEN=<token> claude --name frontend-alice --permission-mode manual
```

O arquivo da chave é `~/.config/symphony/key` no Linux, `~/Library/Application Support/symphony/key` no macOS e `%AppData%\symphony\key` no Windows, ou `key` na pasta que `-data` / `SYMPHONY_DATA` indicar.

O modo de permissão recomendado é o `manual`: no modo `auto`, a maioria das ações não pergunta nada a ninguém, então poucas aprovações chegam à página. Se o bridge não estiver no endereço padrão, defina também `SYMPHONY_BRIDGE`: `http://127.0.0.1:<port>`, `http://localhost:<port>` ou `http://[::1]:<port>` nesta máquina, ou um endereço `https://`; com qualquer outra coisa, o mod nunca entra em contato com o bridge. `SYMPHONY_LANG` escolhe o idioma das mensagens do mod (senão, o idioma do seu sistema; senão, inglês).

Antes de a sessão mandar o token para qualquer lugar, o mod verifica se o bridge naquele endereço conhece o token, sem enviá-lo (`POST /api/hello`). Quando o bridge não pode receber a sessão, a sessão mostra uma destas mensagens, as aprovações dela ficam no terminal, e ela nunca fica travada:

| Mensagem | O que significa |
| --- | --- |
| `não foi possível conectar ao bridge` / `can't reach the bridge` | Nada responde no endereço do bridge, e iniciar o bridge não resolveu (ou o endereço não é desta máquina). |
| `não foi possível iniciar o bridge (...): <reason>` / `couldn't start the bridge ...: <reason>` | Nada respondeu, e o `symphony -background` falhou pelo motivo mostrado (por exemplo, o bridge desta pasta de dados roda em outro endereço: coloque esse endereço em `SYMPHONY_BRIDGE`). |
| `o bridge não reconhece este SYMPHONY_TOKEN` / `the bridge doesn't know this SYMPHONY_TOKEN` | O bridge não tem nenhuma conta com este token: ela foi apagada, ou o bridge usa outra pasta de dados. |
| `não é um bridge do symphony em que esta sessão possa confiar` / `isn't a symphony bridge this session can trust` | Algo responde no endereço, mas não consegue provar que conhece o token: outro programa, ou um symphony mais antigo que o mod (atualize-o). O token não é enviado. |
| `SYMPHONY_BRIDGE (...) precisa estar nesta máquina ou usar https` / `SYMPHONY_BRIDGE ... must be on this machine or use https` | O endereço não é um dos que o mod aceita; o bridge nunca é contatado. |
| `esta conta já tem uma sessão online` / `this account already has a session online` | Uma conta atende uma sessão por vez; crie outra conta para esta sessão. |
| `aguardando sua resposta na página do escritório` / `waiting for your answer in the office page` | Um pedido de permissão ou uma pergunta espera na página do escritório, não no terminal; responda lá, ou aperte Esc para cancelar. Aparece uma vez por sessão. |

Quando nada responde no endereço do bridge nesta máquina, a sessão inicia o bridge sozinha com `symphony -background -open=false -addr <that address>` (se o `symphony` estiver no seu PATH) e avisa (`bridge (...) iniciado em segundo plano` / `started the bridge ... in the background`); o bridge continua rodando depois que a sessão termina. Defina `SYMPHONY_DATA` se as suas contas não estiverem na pasta de dados padrão. Se o bridge desta pasta de dados já roda em outro endereço, a sessão avisa e volta a usar o terminal: coloque esse endereço em `SYMPHONY_BRIDGE`.

## Tarefas em equipe

Uma tarefa é uma equipe de 2 a 5 sessões trabalhando juntas numa mesma coisa: um chat em grupo em Colegas, uma sala de reunião pequena no escritório em pixels. As tarefas precisam do `git`.

**Comece uma tarefa**: clique em "+ Grupo" (no escritório em pixels, numa sala de reunião pequena livre ou em "Nova tarefa" na barra) e descreva o que precisa ser feito. Marque os membros na ordem que quiser (de 2 a 5), escolha o modelo e o esforço de raciocínio de cada um e o modo de permissão da equipe (as próprias configurações do Claude Code por padrão; um funcionário que já trabalhou antes começa com o último modelo e esforço que usou; escolher `bypassPermissions` mostra o aviso), e clique em "Começar a tarefa". Um membro pode ser:

- **Um funcionário**: ganha uma segunda sessão só para a tarefa, que trabalha na pasta do funcionário; a sessão dele na mesa fica como estava. Um funcionário participa de uma tarefa por vez, e quem nunca trabalhou ainda não tem pasta: clique antes uma vez em "Começar a trabalhar", ou dê uma pasta a ele em Configurações → Colegas → "Todos os colegas" (Funcionários no escritório de pixels).
- **Um consultor**: um participante avulso criado para a tarefa com os campos de sempre (nome, pasta, modelo, esforço de raciocínio). Não tem mesa (no escritório em pixels, chega pelo elevador) e vai embora quando a tarefa termina.

**O líder**: por padrão o primeiro membro lidera: divide a tarefa, distribui as partes com SendMessage, revisa o trabalho, mescla os branches dos outros no seu e conta para você o que foi feito. Desmarque a caixa do líder e todos recebem a tarefa inteira e a dividem entre si. Nos dois casos, o bridge manda a cada membro o primeiro prompt: quem está na equipe (os membros se encontram pelo nome com SendMessage), o branch de cada um e as regras.

**Git**: cada membro trabalha no próprio worktree do git, no próprio branch `symphony/<task id>/<member>` (por exemplo `symphony/T1/bob`; um segundo membro com o mesmo nome ganha `-2`) da sua pasta, então ninguém mexe nos arquivos de ninguém e a sua cópia de trabalho fica como estava. Os worktrees ficam em `.symphony/worktrees/` dentro da pasta, escondidos do git pelo `.git/info/exclude` (o seu `.gitignore` não é tocado). Quando a tarefa termina, o que um membro não commitou é commitado no branch dele, os arquivos que o git ignora no worktree dele (saída de build, `.env`, `node_modules` e afins) são movidos para `.symphony/kept/<member>-<task id>/` dentro da pasta, e os worktrees são removidos. O symphony nunca apaga `.symphony/kept/`; apague essa pasta quando não precisar mais do que está nela.

- Nos seus próprios repositórios nada é mesclado: os branches ficam para você mesclar; se houver líder, o branch dele reúne o trabalho de todos.
- Uma pasta que não é um repositório git vira um para a tarefa e continua sendo: `git init` e depois um commit de tudo o que há nela, exceto arquivos que costumam guardar segredos ou são volumosos (`.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa*`, `id_ecdsa*`, `id_ed25519*`, `.npmrc`, `.netrc`, `.pypirc`, `node_modules/`, `.DS_Store`), que ficam listados em `.git/info/exclude`. Esses arquivos continuam na pasta, mas não estão nos worktrees dos membros, então um membro que precise deles instala ou cria os seus. Quando a tarefa termina, os branches dos membros são mesclados no branch da pasta, então a pasta fica com o resultado; o `.git` e os branches são mantidos. Se a tarefa não conseguir começar, a pasta volta a ser uma pasta comum.
- Uma mesclagem que falha é desfeita e a base fica como estava; os branches ficam, com o trabalho feito neles, e a janela da tarefa mostra qual branch deu conflito. Qualquer outra coisa que falhe enquanto uma tarefa termina fica como está, e a janela da tarefa mostra o que falhou.

**Em Colegas**: a tarefa é um chat em grupo abaixo dos colegas, na lista; clique nele para abrir a janela da tarefa no meio: a tarefa, os membros com suas pastas e branches, o chat da equipe e "Encerrar a tarefa", e à direita os rostos e os estados dos membros. Um clique num membro abre a conversa dele, que leva de volta ao grupo. Um membro esperando por você deixa o grupo vermelho.

**No escritório em pixels**: os membros ficam sentados em volta da mesa da sala durante toda a tarefa, e enquanto isso a cadeira na mesa de trabalho do funcionário fica vazia. A placa da sala mostra o id da tarefa; a luz dela, verde enquanto a sala está livre, fica amarela enquanto a tarefa roda e vermelha enquanto um membro espera sua aprovação ou resposta. Clique na sala para abrir a janela da tarefa: a tarefa, os membros com suas pastas e branches, o chat da equipe e "Encerrar a tarefa". Um clique no boneco de um membro na sala também abre a janela da tarefa; um clique num membro na janela da tarefa abre a janela de chat dele encaixada ao lado, igual à janela de um funcionário, um membro de cada vez (clique em outro membro para trocar). O contador vermelho na barra do topo e a busca abrem um membro de uma tarefa do mesmo jeito.

**O chat da equipe**: a janela da tarefa lista as mensagens que os membros trocam entre si, cada uma com o rosto (no escritório em pixels, o boneco em pixel) e o nome de quem mandou, e o texto começa com `@` e o destinatário. Embaixo fica uma caixa para você mesmo escrever para a equipe: `@bob-T1 …` vai só para esse membro (vários `@names` vão para todos eles; digitar `@` abre uma lista dos membros para escolher com as setas, Enter ou um clique), e uma mensagem sem `@` vai para todos da tarefa. Enter envia, Shift+Enter quebra a linha.

Uma tarefa termina quando você a encerra na janela da tarefa ou quando a última sessão dela termina: o bridge para as sessões dos membros, cuida da parte do git como descrito acima e apaga os consultores. Uma tarefa que ainda está rodando quando o bridge para é encerrada do mesmo jeito na próxima vez que o bridge iniciar.

## Contas e mesas

- No symphony, uma identidade é um nome, um token e uma mesa fixa. Uma conta atende uma sessão por vez, mais uma na tarefa de que participa; qualquer outra sessão é recusada, e as aprovações dela ficam no terminal.
- Configurações → Colegas → "Todos os colegas" (Funcionários no escritório de pixels) lista todos os funcionários (um consultor pertence à sua tarefa e não aparece ali) com aquilo com que cada um começa pela página, permite mudar isso para um ou para vários, importa de uma lista e exporta; nunca mostra um token (`POST /api/accounts` devolve um, uma vez só).
- Os logins do Claude não são gerenciados aqui: todas as sessões podem compartilhar o mesmo login do Claude.
- Os dados ficam em `~/.config/symphony` (Linux), `~/Library/Application Support/symphony` (macOS) ou `%AppData%\symphony` (Windows), ou onde `-data` / `SYMPHONY_DATA` indicar. O `accounts.json` guarda os tokens e o `key`, a chave das páginas; só o dono pode ler os dois (0600); no Windows, quem os protege são as permissões da pasta do usuário.

## Custo e permissões

- Cada sessão que o symphony inicia é uma sessão do Claude Code e gasta o seu plano do Claude ou os seus créditos de API como qualquer outra. O Opus no esforço máximo consome isso rápido, e uma tarefa roda de 2 a 5 sessões ao mesmo tempo.
- `bypassPermissions` executa todo comando e toda edição sem perguntar. O que uma sessão lê (um arquivo, uma página web, a saída de uma ferramenta) pode então fazê-la executar comandos em seu nome. As caixas de diálogo de novo funcionário e de nova tarefa mostram um aviso em vermelho quando você o escolhe; por padrão, elas seguem as suas configurações do Claude Code.
- Nas sessões iniciadas pela página, o bridge responde por você à pergunta "trust this folder" do Claude Code, então só inicie sessões em pastas em que você confia.
- "Permitir nesta sessão" vale até a sessão terminar e só deixa passar o que o Claude Code teria perguntado, dentro das suas configurações de permissão: nunca um comando encadeado a outro (`;`, `&&`, `|`, um redirecionamento), e nunca por cima de uma regra de negação (deny) ou das configurações da sua organização. Uma regra de comando cobre todo comando que começa do mesmo jeito (`npm test` cobre `npm test -- --watch`).

## Segurança

A página pode iniciar sessões, o que significa executar comandos nesta máquina, então o bridge controla quem chega até ele:

- **A chave das páginas**: o escritório e a API dele precisam da chave das páginas, um cookie que o seu navegador recebe pelo link que o `symphony` abre. A chave é o arquivo `key` na pasta de dados, que só você pode ler; programas a apresentam no cabeçalho `X-Symphony-Key`.
- **Só esta máquina**: o bridge só escuta e só aceita conexões desta máquina. Ele recusa requisições endereçadas a qualquer outro host (DNS rebinding) e requisições de escrita que outros sites enviem pelo seu navegador, e a API dele não responde a nenhuma requisição que o seu navegador diga vir de outro site ou de outra porta, nem mesmo a um link. Outros sites não conseguem colocar as páginas dele num frame. As páginas só carregam os arquivos do próprio bridge (uma Content Security Policy).
- **O mod verifica o bridge** antes de mandar o token, e o programa `symphony` o verifica antes de apresentar a chave das páginas, então outra coisa escutando no endereço do bridge não recebe nem um nem outro.
- **Contra o que ele não protege**: programas rodando com o seu próprio usuário conseguem ler a chave das páginas e os tokens na pasta de dados, como qualquer arquivo seu. E os navegadores mandam os cookies de um site para todas as portas dele: um servidor que outro usuário desta máquina rode em `127.0.0.1` recebe o cookie da chave das páginas quando o seu navegador o abre, ou quando ocupa a porta do bridge enquanto o bridge está parado e o escritório continua aberto numa aba. Numa máquina compartilhada, feche o escritório antes de parar o bridge e, se a chave pode ter vazado, apague o arquivo `key` na pasta de dados (o próximo início cria uma nova).
- Os programas não são assinados. O macOS pode recusar uma cópia baixada pelo navegador por não conseguir verificar o desenvolvedor: clique em "Abrir Mesmo Assim" em Ajustes do Sistema → Privacidade e Segurança, ou rode uma vez `xattr -d com.apple.quarantine symphony`. No Windows, o SmartScreen pode bloquear uma cópia extraída com "Extrair tudo" do Explorador de Arquivos ao abrir com clique duplo: clique em "Mais informações" e depois em "Executar assim mesmo" (o `Expand-Archive`, que a instalação usa, não deixa nela a marca do download). O `checksums.txt` da versão traz o SHA-256 de cada arquivo, e todo arquivo vem com o atestado do GitHub de onde foi compilado; o passo 2 da instalação verifica os dois.

Para relatar uma vulnerabilidade, veja o [SECURITY.md](SECURITY.md).

## Plataformas

| Sistema | Sessões iniciadas pela página | "Abrir terminal" | Testes |
| --- | --- | --- | --- |
| Linux | Um pseudoterminal dentro do bridge | Um terminal gráfico (x-terminal-emulator, gnome-terminal, konsole, xterm) rodando `symphony attach` | Testes de ponta a ponta completos |
| macOS | Um pseudoterminal dentro do bridge | Terminal.app rodando `symphony attach` | Testes de ponta a ponta completos |
| Windows | Um pseudoconsole (ConPTY) dentro do bridge | Uma nova janela de console rodando `symphony attach` | Testes unitários e os testes de ponta a ponta que não precisam de tmux; sessões reais verificadas no Windows 11 |

O bridge confirma por você que a pasta é confiável ("trust this folder") em todas as plataformas.

## Desenvolvimento

```bash
go test -race ./...                         # testes unitários do bridge e das traduções
claude plugin test mods/symphony-bridge     # testes unitários do mod
claude plugin validate .                    # verifica o marketplace
claude plugin validate mods/symphony-bridge  # verifica o mod
SYMPHONY_E2E=1 go test -tags e2e -v -count=1 -timeout 20m ./e2e/   # ponta a ponta: inicia sessões de verdade e gasta tokens
```

- Para contribuir: o [CONTRIBUTING.md](CONTRIBUTING.md) traz a preparação do ambiente, as verificações a rodar antes de um pull request e as regras; o [CLAUDE.md](CLAUDE.md) é o mapa detalhado do código.
- Para mexer no mod: `go run ./cmd/bridge -mod mods/symphony-bridge`, e as sessões iniciadas pela página recebem a sua cópia do mod com `--plugin-dir`; nas sessões iniciadas à mão, adicione você mesmo `--plugin-dir mods/symphony-bridge`.
- Para mexer nas páginas: `go run ./cmd/bridge -web web`, e basta recarregar para ver suas alterações.
- Para ver o escritório sem sessões de verdade: inicie um segundo bridge com uma pasta de dados própria e alimente-o com atividade fictícia: `D=$(mktemp -d)`, `go run ./cmd/bridge -addr 127.0.0.1:7799 -data "$D"` e depois, em outro terminal (com o mesmo `D`), `go run ./cmd/demo -base http://127.0.0.1:7799 -data "$D"` (a demo lê a chave das páginas do bridge nessa pasta de dados).
- As capturas de tela deste README: `SYMPHONY_E2E=1 SYMPHONY_E2E_DOCS=1 go test -tags e2e -run TestDocsScreens ./e2e/` tira todas de novo com atividade fictícia (sem gastar tokens).
- Traduções: cada idioma é um arquivo, `web/locales/<lang>.json` para as páginas e `internal/i18n/locales/<lang>.json` para o bridge, com as mesmas chaves do `en.json`; as poucas mensagens do mod ficam em `mods/symphony-bridge/hooks/messages.ts`. Os testes unitários verificam se cada idioma tem todas as chaves e os mesmos `{placeholders}`, e o teste de ponta a ponta `TestI18n` abre as páginas em todos os idiomas.
- Os testes de ponta a ponta precisam de claude, tmux (para as sessões que os próprios testes controlam) e Chromium ou Chrome, então rodam no Linux e no macOS. `TestOffice` e `TestModStartsBridge` iniciam sessões de verdade e gastam tokens; os outros não. Por padrão, eles usam o mod deste repositório; defina `SYMPHONY_E2E_MOD=installed` para usar o mod instalado. As capturas de tela vão para `e2e/artifacts/`.
- Versões: aumente a versão em `mods/symphony-bridge/.claude-plugin/plugin.json`, adicione uma seção `## vX.Y.Z (date)` ao `CHANGELOG.md`, leve a dev para a main, depois marque esse commit da main com a tag e envie a tag (`git tag -a v1.0.0 -m v1.0.0 && git push origin v1.0.0`); o GitHub Actions verifica se a tag é a versão do `plugin.json` e se a seção existe, compila o programa para todas as plataformas, atesta onde os arquivos foram compilados e publica uma versão cujas notas começam com essa seção (`.github/workflows/release.yml`; um sufixo como `-rc.1` gera uma pré-versão); numa versão que não é pré-versão, ele em seguida faz commit da fórmula do Homebrew e do manifesto do Scoop na main (e na dev, quando isso é um fast-forward), então faça pull das duas depois. `.github/release.sh v1.0.0 dist` gera os mesmos arquivos localmente, idênticos byte a byte no Linux.

| Pasta | O que é |
| --- | --- |
| `mods/symphony-bridge/` | O mod do Claude Code (TypeScript) |
| `.claude-plugin/marketplace.json` | O marketplace de plugins que o `claude plugin install` lê |
| `cmd/bridge/`, `internal/bridge/` | O bridge |
| `internal/i18n/` | Detecção de idioma e as traduções do bridge |
| `web/` | As páginas do escritório e de aprovações, com suas traduções, embutidas no bridge |
| `cmd/demo/` | Atividade fictícia para o escritório |
| `e2e/` | Testes de ponta a ponta |
| `docs/` | O protocolo entre o bridge e as páginas, e as capturas de tela |

## Licença

[MIT](LICENSE)

O symphony é um projeto independente. Não é afiliado à Anthropic, nem endossado ou patrocinado por ela. Claude e Claude Code são marcas comerciais da Anthropic, PBC.
