# Regras da v3.0 (aposentadas)

Estes arquivos (`rules.json`, `presets.json`) eram usados pela versão PowerShell v3.0
e **não têm mais efeito**. O motor atual (app Go) embute as regras via `go:embed` a
partir de:

    app/internal/engine/assets/rules.json    <- ÚNICO arquivo de regras real
    app/internal/engine/assets/presets.json

Editar os arquivos desta pasta **não muda nada** no produto. Foram movidos para cá em
jul/2026 justamente para eliminar o risco de alguém editar o arquivo errado.
