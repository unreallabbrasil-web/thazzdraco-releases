//go:build windows

package winutil

import (
	"os"
	"path/filepath"
)

// DataDir devolve a pasta de dados do app (%ProgramData%\ThazzDraco), criando-a
// se preciso. Cai para LOCALAPPDATA e so entao para o TEMP — nesta ordem porque
// o TEMP e justamente o que a regra de limpeza varre, e a rede de seguranca
// (historico de undo, backups, estado da sessao) nao pode morar la.
func DataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = os.Getenv("LOCALAPPDATA")
	}
	if base == "" {
		base = os.TempDir()
	}
	d := filepath.Join(base, "ThazzDraco")
	os.MkdirAll(d, 0o755)
	return d
}

// LogOperacao acrescenta linhas ao log de operacoes do app
// (%ProgramData%\ThazzDraco\operacoes.log) — o mesmo arquivo em que o motor
// registra o que aplicou. Tudo que APAGA arquivo escreve aqui: e o unico
// registro do que sumiu, ja que exclusao nao tem undo.
func LogOperacao(linhas ...string) {
	f, err := os.OpenFile(filepath.Join(DataDir(), "operacoes.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, l := range linhas {
		f.WriteString(l + "\r\n")
	}
}

// WriteFileAtomic grava num arquivo temporario e renomeia por cima (atomico no
// NTFS). Evita deixar o JSON truncado/corrompido se o processo morrer no meio da
// escrita — protege o historico de undo, os backups e o estado da sessao, que
// sao exatamente o que precisa sobreviver a uma falha parcial.
func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
