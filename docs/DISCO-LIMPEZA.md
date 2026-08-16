# Ação no explorador de disco — plano

Como o explorador de disco deixa de só **mostrar** e passa a **liberar espaço**, sem virar um
apagador de arquivos do cliente.

> **Status:** plano. Nada implementado. Este documento existe porque esta é a parte mais perigosa do
> app: é a única que **apaga arquivos**, e apagar não tem undo. O motor de regras sempre pôde
> desfazer o que fez; aqui, não. Por isso a discussão vem antes do código.

Referências: [SESSAO](SESSAO.md) (o plano e o consentimento) · [RULES-SCHEMA](RULES-SCHEMA.md) ·
[GUIA-OTIMIZACAO-GAMING](GUIA-OTIMIZACAO-GAMING.md) (invariantes).

---

## 1. A linha que não se cruza

O produto tem um invariante que vale mais que qualquer GB liberado:

> **Nunca toca em documentos pessoais.**

Então o plano separa o mundo em três, e a diferença entre eles é o que o app **pode fazer**:

| Classe | O que é | O que o app faz |
|---|---|---|
| **Cache regenerável** | Some e o programa recria sozinho, sem o usuário perder nada | Oferece limpar, com confirmação |
| **Dado do usuário** | Downloads, modelos de IA baixados, documentos, saves | **Só mostra o tamanho.** Nunca apaga |
| **Não mexer** | Coisas que *parecem* lixo e quebram o sistema se sumirem | Mostra o tamanho e **explica por que não dá** |

A terceira classe é a que separa uma ferramenta séria de um "limpador de PC" de propaganda —
ver §4.

---

## 2. O que existe hoje na sua máquina

Medido com o explorador, não estimado (C: com 429 GB usados e **41,7 GB livres**):

### Dá para limpar (cache regenerável)

| Categoria | Tamanho | O que acontece se apagar |
|---|---:|---|
| Cache do **npm** | **6,00 GB** | Próximo `npm install` rebaixa o que precisar |
| Cache do **pip** | **4,50 GB** | Próximo `pip install` rebaixa |
| Cache de build do **Go** | 0,11 GB | Próxima compilação refaz (fica mais lenta uma vez) |
| Cache de shaders (NVIDIA/D3D) | 0,84 GB | Já coberto pela **Limpeza profunda** — atalho, não duplicar |
| Temporários do usuário | 1,27 GB | Já coberto pela regra de limpeza existente |
| **Subtotal do que é novo** | **~10,6 GB** | |

### Só relatório (dado do usuário — o app não apaga)

| Achado | Tamanho | Por que não apago |
|---|---:|---|
| Modelos do **Hugging Face** | 3,82 GB | Modelos que você baixou de propósito. Re-baixar leva muito tempo |
| Modelos do **Ollama** | 1,88 GB | Idem — é acervo, não cache |
| **Downloads** | 2,20 GB (255 arq.) | Pasta pessoal. Mostro os instaladores velhos e você decide |
| **VHDX duplicado** | **8,73 GB** | Duas cópias do mesmo disco de máquina virtual (`Roaming\Claude` e `Local\Packages\Claude…`). Aponto as duas; quem apaga é você |
| `AppData\Local\Packages` | 27,09 GB | Dados de apps da Store. Apagar em bloco quebra apps |
| `.claude` | 3,34 GB | Dados de aplicativo |

### Não mexer (parece lixo, não é)

| Pasta | Tamanho | Por que fica |
|---|---:|---|
| `C:\Windows\Installer` | **2,56 GB** | Cache do MSI. Some → **desinstalar e reparar programas para de funcionar**. É a armadilha clássica dos "limpadores de PC" |
| `C:\ProgramData\Package Cache` | 0,92 GB | Instaladores do Visual Studio/redistribuíveis usados para reparo |
| `pagefile.sys` | 16,0 GB | Arquivo de paginação em uso pelo Windows. Só se mexe pelo painel de memória virtual |

**Resumo honesto:** dá para liberar **~10,6 GB automático** e apontar mais **~15 GB** que dependem de
decisão sua (duplicado de 8,7 GB + modelos + Downloads). Numa máquina com 41,7 GB livres, é entre
25% e 60% de espaço livre a mais.

---

## 3. Regras de segurança (no código, não na intenção)

1. **O cliente nunca manda caminho.** A API aceita **ids de categoria**; os caminhos são derivados do
   catálogo dentro do Go, na hora de apagar. Não existe endpoint que apague um caminho arbitrário.
2. **Allowlist, não blocklist.** Só apaga o que casa com um caminho do catálogo, já resolvido para o
   perfil do usuário real.
3. **Deny list explícita** por cima (§4): mesmo que um catálogo futuro erre, esses caminhos são
   recusados na porta.
4. **Nunca segue junção/symlink** ao apagar — senão apagar um cache pode sair da pasta.
5. **Apaga arquivo, mantém a pasta** — igual à limpeza que já existe.
6. **Exige `confirmar: true`** e a UI mostra a lista do que vai sumir, com o tamanho, antes.
7. **Registra tudo** em `operacoes.log`: categoria, caminho, bytes liberados.
8. **Nunca entra em pasta pessoal** (Desktop, Documents, Downloads, Pictures, Videos, Music) — nem
   por catálogo, nem por engano.
9. **Sem undo, e a UI diz isso** na cara, com o mesmo peso que a Sessão usa para a limpeza atual.

**Testes que travam o build:** o catálogo não pode conter caminho da deny list; nenhum caminho pode
cair fora do perfil do usuário ou das pastas de sistema permitidas; nenhuma pasta pessoal pode ser
alvo. Isso vira teste de verdade, não comentário.

---

## 4. A deny list (o que *parece* lixo e não é)

Vai no código com o motivo escrito, porque daqui a seis meses alguém vai olhar `C:\Windows\Installer`
com 2,5 GB e querer "otimizar":

| Caminho | Por que é intocável |
|---|---|
| `C:\Windows\Installer` | Cache do MSI: sem ele, desinstalar/reparar programas quebra |
| `C:\ProgramData\Package Cache` | Instaladores usados para reparo do VS/redistribuíveis |
| `C:\Windows\System32`, `SysWOW64`, `WinSxS` | Sistema. WinSxS só via `DISM /StartComponentCleanup` (já existe no Reparo) |
| `pagefile.sys`, `hiberfil.sys`, `swapfile.sys` | Em uso pelo kernel; se mexe por configuração, não por exclusão |
| Perfis de usuário (`Desktop`, `Documents`, `Downloads`, `Pictures`, `Videos`, `Music`) | Dado pessoal |
| Bibliotecas de jogos (Steam/Epic/…) | Apagar aqui é desinstalar jogo pelas costas |
| `System Volume Information` | Pontos de restauração — a própria rede de segurança do app |

---

## 5. Duplicados grandes

O caso dos dois VHDX de 8,73 GB é real e vale a pena, mas **mesmo tamanho não é prova de que são
iguais**. O plano:

1. Durante a varredura, guardar candidatos: arquivos **≥ 256 MB** agrupados por tamanho exato.
2. Para os grupos com 2+ arquivos, confirmar lendo **1 MB do início e 1 MB do fim** de cada e
   comparando o hash. Custa alguns MB de leitura, não o arquivo inteiro.
3. Reportar só o que a confirmação passou, dizendo **como** foi confirmado.
4. **Nunca apagar automaticamente.** O app mostra os dois caminhos e um botão "abrir a pasta" — a
   escolha de qual cópia fica é humana, e a ferramenta não tem como saber qual das duas algum
   programa está usando.

---

## 6. API

| Rota | O que faz |
|---|---|
| `GET /api/disco/achados` | Classifica a varredura já feita: categorias com tamanho **medido**, classe (limpável/relatório/intocável) e duplicados confirmados |
| `POST /api/disco/limpar` | `{ids:[], confirmar:true}` → apaga só as categorias limpáveis, devolve bytes liberados por categoria |
| `POST /api/disco/abrir` | Abre uma pasta do achado no Explorer (só caminhos vindos do próprio achado) |

Nada disso pega o cadeado do motor: é leitura + exclusão de arquivo, não mexe em registro.

---

## 7. Tela

No **Máquina → Disco**, abaixo da árvore, um bloco **"O que dá para liberar"** com três seções
visualmente distintas:

1. **Dá para limpar** — caixas de seleção, tamanho, e o que acontece ao apagar. Rodapé com o total
   selecionado e um botão que abre a confirmação listando tudo.
2. **Decisão sua** — sem caixa de seleção. Duplicados (com os dois caminhos), modelos de IA,
   instaladores velhos em Downloads. Cada um com "abrir pasta".
3. **Não mexo nisto** — recolhido por padrão. Mostra o tamanho e o motivo. É o que impede o técnico
   de ir "otimizar" o `C:\Windows\Installer` por conta própria.

---

## 8. Fatias

| Fatia | Entrega | Aceite |
|---|---|---|
| **L1 ✅** | Catálogo + classificação + tela (**sem apagar nada**) | O bloco mostra as categorias com tamanho real e as classes separadas |
| **L2 ✅** | Duplicados com confirmação por hash de pontas | Os VHDX aparecem confirmados, com os caminhos e "abrir pasta" |
| **L3 ✅** | A limpeza de verdade: allowlist, deny list, log, confirmação | Libera os ~10,6 GB de cache e recusa qualquer caminho fora do catálogo |
| **L4** | Ligar com o resto: atalho para a Limpeza profunda no que já existe, e o resultado indo para o relatório da Sessão | Sem duplicar o que a Limpeza profunda já faz |

### O que a L3 entregou

`internal/winutil/disco_limpeza.go` + `disco_limpeza_test.go`, `POST /api/disco/limpar`, e a barra de
seleção com modal de confirmação na tela do Disco.

**As três camadas ficaram assim, e cada uma foi testada isoladamente:**

| Camada | Teste que a prova |
|---|---|
| Cliente manda **id de categoria**, nunca caminho | Mandar `C:\Windows\System32` como id é recusado como "categoria desconhecida" |
| Só classe **limpável** executa | `windows-installer`, `downloads` e `modelos-hf` são recusados com o motivo, mesmo com `confirmar: true` |
| `podeApagar()` confere cada caminho na porta | 17 caminhos proibidos recusados: raiz de unidade, `C:\Windows`, `Installer`, `WinSxS`, `Program Files`, pasta pessoal, OneDrive, saves, caminho relativo, `..` |

**Mais três invariantes com teste próprio:**

- **A exclusão não atravessa junção.** Um teste cria uma junção dentro do cache apontando para fora e
  confirma que o arquivo de fora sobrevive — é o pior acidente possível aqui.
- **A pasta de cima sobrevive.** Só o conteúdo sai; o `npm-cache` continua existindo, porque o npm
  quebra se a pasta sumir inteira.
- **A cadeia inteira, num perfil falso.** Com `USERPROFILE` apontando para uma pasta temporária, a
  limpeza apaga o cache certo (6.000 bytes em 2 arquivos), mantém a pasta, e **não encosta** no
  `Documents` nem na categoria vizinha que não foi pedida.

**Decisões de comportamento:** nada vem marcado (marcado por padrão não seria consentimento em algo
sem undo); arquivo em uso é **mantido**, nunca forçado, e aparece no resultado como "em uso"; tudo é
registrado em `operacoes.log`; e a tela varre de novo depois de apagar, para os números não ficarem
velhos.

**L1 e L2 não apagam nada** — dá para rodar na sua máquina sem risco nenhum e conferir se a
classificação está certa antes de existir qualquer botão que apague.

---

## 9. Decisões tomadas

1. **Modelos de IA (Hugging Face + Ollama): só relatório.** São GB que o usuário escolheu baixar e
   que levam muito tempo para voltar. O app mostra o tamanho e não oferece botão.
2. **A limpeza de disco NÃO entra no plano da Sessão.** Apagar não tem undo, e item sem undo dentro
   de um lote aprovado em bloco é como se apaga um dia o que não devia. Fica na tela do Disco, com
   confirmação própria. (O resultado ainda pode aparecer no relatório — isso é a L4.)
3. **Instaladores velhos em Downloads: relatório**, com os maiores e mais antigos listados. A pasta é
   pessoal; quem apaga é o dono.

---

## 10. Fatia D — escolher o disco, apagar do app, analisar melhor

Três pedidos de uma vez: poder escolher **qual** unidade varrer, poder **apagar de dentro do
programa**, e uma análise mais forte. As três mexem na mesma tela, mas só a segunda mexe na
segurança — por isso ela tem seção própria.

### 10.1 Escolher o que varrer

A tela herdava a lista do diagnóstico, que filtra `DRIVE_FIXED`. Para "saúde do PC" isso está certo;
para "onde está meu espaço" está errado — **HD externo e pendrive nunca apareciam**, e é justamente
onde mora arquivo esquecido. `winutil.Unidades()` lista tudo com o tipo declarado (fixo, removível,
rede, óptico, RAM), rótulo do volume e sistema de arquivos.

Decisões:

- **Unidade sem mídia continua na lista, apagada.** Sumir a letra faria o técnico que acabou de
  plugar o pendrive achar que o app não viu. Ele vê, e diz o motivo.
- **Rede aparece com aviso.** Varrer `\servidor` depende da rede e pode demorar muito mais; isso
  precisa ser dito *antes* do clique.
- **Dá para varrer uma pasta**, não só a unidade. Varrer 4 TB para olhar uma pasta é desperdício do
  tempo de quem está esperando na frente do PC. `AlvoDeVarredura()` valida e normaliza.

### 10.2 Apagar o que o usuário escolheu

Este é o único caminho do app que apaga arquivo **do usuário**. A limpeza por categoria (§3) e esta
exclusão trocam de garantias de propósito:

| | limpeza por categoria | exclusão escolhida |
|---|---|---|
| quem escolhe | o app, de uma lista fechada | o usuário, item por item, vendo o tamanho |
| o que alcança | só cache regenerável | qualquer dado dele |
| destino | apagado de vez | **Lixeira** por padrão |
| o que é proibido | pasta pessoal | pasta do sistema |

**A Lixeira é a peça central.** Ela transforma a única operação sem volta do produto em uma operação
com volta — e sem depender de o app estar certo sobre o que era importante. Usa `SHFileOperationW`
com `FOF_ALLOWUNDO`, que é a única forma de o arquivo ir para a Lixeira de um jeito que o Windows
saiba restaurar. Apagar de vez existe, mas é escolha explícita no modal, porque arquivo gigante não
cabe na Lixeira e um botão que às vezes tem desfazer e às vezes não é pior que nenhum.

Camadas de segurança, cada uma suficiente sozinha:

1. **Limite de consentimento**: o caminho tem que estar dentro da varredura atual. Sem varredura
   pronta, o endpoint recusa tudo — sem raiz não existe limite.
2. **`PodeExcluirEscolhido`** recusa por padrão: raiz de unidade, Windows, Program Files,
   recuperação, inicialização, pontos de restauração, a própria Lixeira, perfil inteiro
   (`C:\Users\Fulano`), as pastas pessoais conhecidas *em si* (o conteúdo delas pode), junction/link
   e os arquivos que o sistema usa em tempo real (`pagefile.sys` e afins).
3. **A UI não oferece o que o portão recusa.** A árvore e as listas vêm anotadas com
   `protegido`/`motivo` pela *mesma* função que recusa na hora de apagar. Antes dava para marcar
   `C:\Windows`, ver o total subir, confirmar, e só então receber "recusado" — uma interface que
   promete o que não cumpre.
4. **Confirmação que mostra item por item**, com tamanho e caminho, e o destino no próprio rótulo do
   botão ("Mandar 41 GB para a Lixeira" / "Apagar 41 GB de vez").
5. **Tudo em `operacoes.log`**, com destino, tamanho e as recusas.

O resultado é conferido no disco, não no código de retorno: se o item ainda existe depois da
operação, ele conta como falha, mesmo que a API não tenha reclamado.

### 10.3 Análise

- **Por tipo de conteúdo.** A árvore diz *onde*; isto diz *o quê*. "80 GB em `D:\midia`" não ajuda
  ninguém a decidir; "80 GB de vídeo, o maior com 12 GB de 2019" ajuda na hora. Vem da extensão, que
  é o que dá para saber sem abrir o arquivo — a tela mostra como pista, com exemplos reais, nunca
  como veredito.
- **Grandes e parados.** Arquivos acima de 100 MB sem modificação há mais de um ano. É o lugar mais
  provável de sobrar espaço sem fazer falta. Com a ressalva na própria tela: backup e coleção também
  ficam parados de propósito.
- **Idade em todo lugar.** "há 3 anos" decide melhor que "1274 dias".

Custo: a soma por extensão é acumulada por pasta e mesclada uma vez só, não a cada arquivo — travar
um mapa global 1,5 milhão de vezes num disco cheio custaria mais que a varredura inteira.

### 10.4 O que ficou de fora

- **Varredura guardada entre sessões.** Hoje reabrir o app exige varrer de novo. Guardar a árvore
  podada permitiria "cresceu 12 GB desde ontem, quase tudo em X" — que é o que um técnico quer ver.
- **Busca por nome** dentro do resultado (hoje só dá para navegar).
- **L4**: o resultado da limpeza no relatório da Sessão.

---

## 11. Fatia E — o veredito com prova

O relato que originou esta fatia: *"eu nunca sei o que eu posso apagar ou não. Posso apagar esses
caches? Inclusive de npm, java, python? Dados de apps da Store abre uma pasta com várias coisas
dentro, não sei o que pode ou não, se vai quebrar algo. É foda."*

A tela dizia "cache regenerável" e pedia confiança. **Confiança não era o que faltava — faltava
prova.** E o app já tinha os dados para provar; só não os mostrava.

### 11.1 As três evidências

Todo achado passou a carregar as três coisas que respondem "posso apagar?":

1. **Quando foi usado pela última vez.** Sai de graça: a varredura já lê a data de cada arquivo.
   Cada nó da árvore ganhou `Recente` (dias desde o arquivo mais novo abaixo dele), propagado de
   baixo para cima — uma pasta cujo neto mudou hoje foi usada hoje.
2. **Quem é o dono, e se o dono existe.** `exec.LookPath` para ferramentas de linha de comando; o
   registro de pacotes para apps da Store. Se o Cargo não está instalado, o cache dele é peso morto
   puro: não há o que "rebaixar depois" porque não há quem baixe.
3. **Como volta se apagar.** "volta sozinho no próximo `npm install`" é uma coisa; "não volta: é
   baixar 3,8 GB de novo" é outra bem diferente, e a diferença é o que decide na prática.

O veredito (`SEGURO` / `PROVÁVEL` / `CUIDADO` / `NÃO`) **nunca aparece sozinho** — vem sempre com os
fatos que o produziram. Selo sem prova seria só mais um aviso, e aviso é exatamente o que já existia
e não resolvia.

### 11.2 A regra que não se dobra

Cache é sempre `SEGURO`, porque por definição ele se refaz — a evidência muda o **custo**, não o
**se**. Dado do usuário **nunca** vira `SEGURO` automaticamente, por mais velho que esteja: no
máximo `PROVÁVEL`, com a decisão devolvida a quem é dono do arquivo. Isso está preso por teste.

### 11.3 Quebrar o agregado

"Dados de apps da Store — 26,8 GB" era o pior item da tela: abre uma pasta com 134 nomes ilegíveis e
ninguém sabe o que é o quê. Agora abre item por item, e a lista responde sozinha:

```
12,5 GB  Claude          CUIDADO   instalado, usado hoje — apagar zera login e configuração
11,2 GB  SpotifyMusic    CUIDADO   instalado, usado hoje
 500 MB  TranslucentTB   PROVÁVEL  o aplicativo não está mais instalado — sobra de desinstalação
```

A chave é cruzar `%LOCALAPPDATA%\Packages` com as famílias de pacote instaladas
(`HKCU\...\AppModel\Repository\Packages`, montando `Nome_PublisherId` a partir do nome completo).
**A pasta de dados não some quando o app é desinstalado** — então essa diferença é espaço parado que
ninguém consegue enxergar pelo Explorer.

Cada item do detalhamento é um caminho real: dá para mandar para a Lixeira pela mesma cesta,
passando pelo mesmo portão de exclusão da §10.2.

### 11.4 O que fica de fora, e por quê

- **Nada aqui apaga sozinho.** O veredito informa; quem marca é o usuário.
- **`SEGURO` não vira "marcado por padrão".** Um item pré-marcado numa lista que apaga arquivo não é
  consentimento, é armadilha.
- **A extensão e o registro são pistas, não verdade absoluta.** Um app pode guardar dados fora da
  pasta dele; um pacote pode estar provisionado sem estar instalado para este usuário. Por isso o
  veredito máximo para dado do usuário continua sendo `PROVÁVEL`.
