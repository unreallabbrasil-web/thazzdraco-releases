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
