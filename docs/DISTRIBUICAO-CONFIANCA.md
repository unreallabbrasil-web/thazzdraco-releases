# Distribuição & Confiança — por que o Windows alerta e como resolver

> Guia honesto sobre os avisos de **SmartScreen** ("O Windows protegeu o computador") e
> **Defender** ("contém um vírus ou software possivelmente indesejado") que aparecem ao abrir o
> `ThazzDraco.exe` num PC novo — a causa real, o que já foi feito de graça, e o caminho definitivo.

---

## 1. O que está acontecendo (são DOIS alertas diferentes)

### 1.1 Tela roxa — SmartScreen ("O Windows protegeu o computador")
É **reputação**, não vírus. O Microsoft Defender SmartScreen bloqueia todo `.exe` que (a) veio da
internet/transferência (tem a "marca da web") e (b) **não tem assinatura digital com reputação**.
Como o `ThazzDraco.exe` é um binário Go novo, sem certificado, sua reputação é zero → tela roxa em
qualquer PC onde ele ainda não foi visto. **Só some de verdade com assinatura de código** (ver §4) ou
com os paliativos de campo (§5).

### 1.2 Caixa vermelha — Defender ("contém um vírus / software possivelmente indesejado")
Essa é a séria: o antivírus **bloqueou e colocou em quarentena**. A verdade sem rodeios: o ThazzDraco
aciona vários gatilhos heurísticos de malware **ao mesmo tempo**, porque faz de verdade as coisas que
malware também faz:

| Comportamento legítimo do app | Como a heurística do AV lê |
|---|---|
| `.exe` sem assinatura + `requireAdministrator` | "pede admin sem identidade conhecida" |
| Embute e extrai o `PresentMon.exe` em runtime | "dropper" (solta outro executável) |
| `Add-MpPreference -ExclusionPath` (excluir jogo do Defender) | 🚨 "programa tentando se excluir do antivírus" — clássico de malware |
| Edita registro/serviços/powercfg em massa como admin | "modifica o sistema profundamente" |
| Chama PowerShell (debloat, exclusão, scan) | "spawn de PowerShell" |
| É literalmente um **"Otimizador"** | categoria **PUA** (a Microsoft marca "otimizadores/boosters" por padrão) |

Nenhum desses sozinho é malware. **Juntos**, formam o retrato exato que a heurística procura.

> **Descubra o nome da detecção.** Em **Segurança do Windows → Proteção contra vírus e ameaças →
> Histórico de proteção**, clique na detecção. O nome diz a estratégia:
> - `PUA:Win32/...` → é a categoria "otimizador". Resolve com submissão (§3) + assinatura (§4).
> - `Trojan:Win32/Wacatac`, `Program:Win32/...`, `Zpevdo`, etc. → **falso-positivo heurístico**.
>   Submeter à Microsoft (§3) resolve.

---

## 2. O que já foi feito de graça (commitado no projeto)

Reduz a probabilidade e a gravidade da detecção — **não elimina** (só assinatura elimina o SmartScreen):

1. **Metadados de versão no `.exe`** (`resgen.py` → recurso `RT_VERSION`). Antes a aba *Detalhes* do
   arquivo era em branco (sinal clássico de suspeito). Agora traz **Empresa, Produto, Descrição,
   Versão e Copyright**. Conferir: `(Get-Item .\ThazzDraco.exe).VersionInfo`.
2. **PresentMon embutido compactado (gzip)** (`internal/fps/fps.go`). Antes o cabeçalho PE (`MZ…`) de
   um `.exe` aparecia cru dentro do nosso binário — sinal forte de "dropper". Agora vai compactado e é
   descompactado em runtime, então o scanner estático não vê um PE embutido. (Bônus: binário ~200 KB
   menor.)

### 2.1 Recomendado fazer a seguir (decisão de produto)
- **Tornar a exclusão do Defender (`Add-MpPreference`) opcional e fora do "⚡ Tudo".** É o gatilho
  comportamental nº 1. O ganho de FPS de excluir a pasta do jogo é pequeno; o custo em reputação é
  alto. Sugestão: deixar **desmarcado por padrão**, com aviso de que pode disparar o antivírus, e
  nunca aplicá-la em lote automático.

---

## 3. Submeter à Microsoft como falso-positivo (GRÁTIS — a correção mais confiável do Defender)

Se a detecção for falso-positivo (o que é o caso), a Microsoft analisa e libera — normalmente em
**24–72h**, na nuvem (vale para todos os PCs com Defender).

1. Acesse **https://www.microsoft.com/en-us/wdsi/filesubmission**.
2. Escolha **"Software developer"** (você é o autor do app).
3. Faça login com conta Microsoft.
4. Envie o **`dist/ThazzDraco.exe`** atual.
5. Em "Detection name", informe o nome que apareceu no Histórico de proteção (§1.2).
6. Na justificativa, explique em inglês objetivo, algo como:
   > "ThazzDraco Optimizer is a legitimate Windows gaming optimization utility I develop. It is a
   > false positive. It modifies registry/services/power settings with user consent and full undo,
   > and bundles Intel PresentMon (MIT) to measure FPS via ETW. No malicious behavior."
7. Marque que acredita ser **falso-positivo** e envie.

> ⚠️ Sem assinatura, a liberação vale para **aquele hash**. Toda vez que você **recompilar**, o hash
> muda e pode voltar a ser pego — aí reenvia. **Com** certificado (§4), a Microsoft libera pelo
> **certificado**, não pelo hash → some o reenvio a cada build. Por isso assinar é o destino final.

---

## 4. A correção definitiva: assinatura de código (quando houver orçamento + CNPJ)

É o que CCleaner, MSI Afterburner, etc. usam. Resolve o SmartScreen de vez e quase zera a heurística.

| Tipo | Custo aprox./ano | SmartScreen | Requisitos |
|---|---|---|---|
| **EV (Extended Validation)** | US$ 250–500 | **Reputação INSTANTÂNEA** (some no 1º PC) | CNPJ + token físico ou HSM na nuvem |
| **OV (comum)** | US$ 150–250 | Reputação **acumulada** ao longo de downloads (no começo ainda avisa) | CNPJ + HSM |

- Emissoras: SSL.com, Sectigo, DigiCert, GlobalSign, Certera (revendas saem mais barato).
- **Exige entidade legal** (CNPJ). Um **MEI** costuma ser aceito por várias CAs.
- Desde 2023 a chave precisa ficar em **hardware/HSM** (token USB ou assinatura na nuvem tipo Azure
  Key Vault). Há serviços de "cloud signing" que evitam o token físico.
- Depois de ter o certificado: assinar no `build.ps1` com
  `signtool sign /fd SHA256 /tr http://timestamp.<ca> /td SHA256 /a dist\ThazzDraco.exe`. Quando você
  decidir seguir por aqui, eu integro isso ao build automaticamente.

**Recomendação:** assim que o trabalho com clientes justificar, ir de **EV** — é o único que mata o
SmartScreen no primeiro PC, que é exatamente a vergonha na frente do cliente.

---

## 5. Playbook de campo — o técnico libera rápido e sem improviso

Enquanto não há assinatura, **o técnico vai precisar liberar manualmente** em PCs novos. Faça isso
com naturalidade e profissionalismo — é o mesmo procedimento de qualquer ferramenta técnica não
assinada (DDU, Afterburner beta, etc.).

### 5.1 Antes de levar (no seu PC)
- Distribua em **pendrive formatado em exFAT ou FAT32**. Arquivos copiados de FAT **não carregam** a
  "marca da web" → **mata a tela roxa do SmartScreen** (não mata o Defender).
- Se for por TeamViewer/Parsec/download: no arquivo → **botão direito → Propriedades → marcar
  "Desbloquear" → OK** (ou PowerShell: `Unblock-File .\ThazzDraco.exe`).
- Sempre leve o **mesmo `.exe`** (mesmo hash) já submetido à Microsoft (§3) — não recompile sem
  reenviar.

### 5.2 Se aparecer a tela roxa (SmartScreen)
1. Clique em **"Mais informações"**.
2. Clique em **"Executar assim mesmo"**.

### 5.3 Se o Defender colocar em quarentena ("contém um vírus")
1. **Segurança do Windows → Proteção contra vírus e ameaças → Histórico de proteção**.
2. Abra a detecção do ThazzDraco → **Ações → Permitir** (restaura da quarentena).
3. Rode o `ThazzDraco.exe` de novo → **Executar como administrador**.

   *Alternativa (antes de baixar/copiar):* adicionar uma **exclusão de pasta** para o local do `.exe`
   em **Proteção contra vírus e ameaças → Gerenciar configurações → Exclusões → Adicionar pasta**.
   Use com critério — explique ao cliente o que está fazendo e por quê.

### 5.4 Frase pronta para o cliente (transparência gera confiança)
> "O Windows avisa porque o programa não é de uma loja grande e mexe a fundo no sistema pra otimizar —
> é uma ferramenta técnica, não um app de loja. Vou liberar aqui, com sua autorização; tudo que ele
> faz é reversível e fica registrado."

---

## 6. Resumo do caminho

| Prazo | Ação | Mata o quê | Custo |
|---|---|---|---|
| **Agora** | Metadados + gzip (feito) | Reduz heurística do Defender | grátis |
| **Agora** | Submeter à Microsoft (§3) | Detecção do Defender (falso-positivo) | grátis |
| **Agora** | Pendrive exFAT + Desbloquear (§5) | Tela roxa do SmartScreen | grátis |
| **Agora** | Exclusão do Defender opcional (§2.1) | Reduz gatilho comportamental | grátis |
| **Quando puder** | Certificado **EV** + assinar no build (§4) | **SmartScreen + heurística de vez** | ~US$ 250–500/ano + CNPJ |

> **A verdade:** software que mexe no sistema como administrador só é confiável de verdade quando é
> **assinado e tem reputação**. Os passos grátis reduzem muito o atrito e dão um procedimento limpo
> ao técnico — mas a solução que faz o alerta **não aparecer em nenhum PC** é a assinatura de código.
</content>
</invoke>
