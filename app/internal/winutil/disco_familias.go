//go:build windows

package winutil

import (
	"path/filepath"
	"sort"
	"strings"
)

// Agrupamento por TIPO de conteudo.
//
// A arvore de pastas responde "onde esta o espaco". Ela nao responde "o que e".
// E a decisao de apagar quase nunca e por lugar: e por tipo de coisa. "80 GB em
// D:\midia" nao ajuda ninguem; "80 GB de video, e o maior tem 12 GB de 2019"
// ajuda na hora.
//
// A tabela e por extensao porque e o que da para saber sem abrir o arquivo.
// Extensao mente as vezes (um .bin pode ser qualquer coisa), entao a UI mostra
// isto como PISTA, com exemplos reais que o usuario reconhece — nunca como
// veredito.

type somaExt struct {
	bytes    int64
	arquivos int64
	maior    ArquivoGrande
}

// familias: extensao -> id da familia. Ordem nao importa; o mapa e consultado.
var familias = map[string]string{
	// vídeo
	"mp4": "video", "mkv": "video", "avi": "video", "mov": "video", "wmv": "video",
	"flv": "video", "webm": "video", "m4v": "video", "mpg": "video", "mpeg": "video",
	"ts": "video", "m2ts": "video", "vob": "video",
	// imagem de disco / máquina virtual
	"iso": "imagem-disco", "img": "imagem-disco", "vhd": "imagem-disco", "vhdx": "imagem-disco",
	"wim": "imagem-disco", "esd": "imagem-disco", "nrg": "imagem-disco", "mdf": "imagem-disco",
	"vmdk": "imagem-disco", "vdi": "imagem-disco", "qcow2": "imagem-disco", "hdd": "imagem-disco",
	// instalador
	"exe": "instalador", "msi": "instalador", "msix": "instalador", "appx": "instalador",
	"msu": "instalador", "cab": "instalador",
	// compactado
	"zip": "compactado", "rar": "compactado", "7z": "compactado", "tar": "compactado",
	"gz": "compactado", "bz2": "compactado", "xz": "compactado", "zst": "compactado",
	// foto e imagem
	"jpg": "imagem", "jpeg": "imagem", "png": "imagem", "gif": "imagem", "bmp": "imagem",
	"tif": "imagem", "tiff": "imagem", "webp": "imagem", "heic": "imagem", "raw": "imagem",
	"cr2": "imagem", "nef": "imagem", "arw": "imagem", "dng": "imagem", "psd": "imagem",
	"svg": "imagem", "ico": "imagem",
	// áudio
	"mp3": "audio", "wav": "audio", "flac": "audio", "aac": "audio", "ogg": "audio",
	"m4a": "audio", "wma": "audio", "opus": "audio",
	// documento
	"pdf": "documento", "doc": "documento", "docx": "documento", "xls": "documento",
	"xlsx": "documento", "ppt": "documento", "pptx": "documento", "txt": "documento",
	"odt": "documento", "ods": "documento", "csv": "documento", "rtf": "documento",
	"epub": "documento", "mobi": "documento",
	// backup
	"bak": "backup", "bkp": "backup", "old": "backup", "backup": "backup",
	"vbk": "backup", "tib": "backup", "gho": "backup", "spf": "backup",
	// banco de dados
	"db": "banco", "sqlite": "banco", "sqlite3": "banco", "ldf": "banco", "sql": "banco",
	"mdb": "banco", "accdb": "banco",
	// log e despejo de erro
	"log": "log", "dmp": "log", "mdmp": "log", "etl": "log", "evtx": "log", "trace": "log",
	// conteúdo de jogo
	"pak": "jogo", "vpk": "jogo", "gcf": "jogo", "bsa": "jogo", "ba2": "jogo",
	"uasset": "jogo", "upk": "jogo", "wad": "jogo", "sav": "jogo",
	// binário de programa
	"dll": "programa", "sys": "programa", "so": "programa", "lib": "programa",
	"pdb": "programa", "obj": "programa", "o": "programa", "a": "programa",
}

var nomesFamilia = map[string]string{
	"video":        "Vídeo",
	"imagem-disco": "Imagem de disco e máquina virtual",
	"instalador":   "Instaladores e programas",
	"compactado":   "Arquivos compactados",
	"imagem":       "Fotos e imagens",
	"audio":        "Áudio",
	"documento":    "Documentos",
	"backup":       "Backups",
	"banco":        "Bancos de dados",
	"log":          "Logs e despejos de erro",
	"jogo":         "Conteúdo de jogo",
	"programa":     "Bibliotecas e binários",
	"outros":       "Outros",
}

// extensaoDe devolve a extensao em minusculas, sem ponto.
func extensaoDe(nome string) string {
	e := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	// Extensão absurda quase sempre é ponto no meio do nome ("backup.2024.10.31"),
	// não extensão de verdade.
	if len(e) > 12 {
		return ""
	}
	return e
}

func familiaDe(ext string) string {
	if f, ok := familias[ext]; ok {
		return f
	}
	return "outros"
}

// agrupaFamilias converte a soma por extensao no ranking por tipo.
//
// "Outros" so aparece se sobrar algo relevante — e ele fica sempre por ultimo,
// porque uma categoria que quer dizer "nao sei" nao pode encabecar a lista.
func agrupaFamilias(porExt map[string]*somaExt) []FamiliaArquivo {
	acc := map[string]*FamiliaArquivo{}
	for ext, s := range porExt {
		if s == nil || s.bytes <= 0 {
			continue
		}
		id := familiaDe(ext)
		f := acc[id]
		if f == nil {
			f = &FamiliaArquivo{ID: id, Nome: nomesFamilia[id]}
			acc[id] = f
		}
		f.Bytes += s.bytes
		f.Arquivos += s.arquivos
		f.Exemplos = append(f.Exemplos, s.maior)
	}

	out := make([]FamiliaArquivo, 0, len(acc))
	for _, f := range acc {
		sort.Slice(f.Exemplos, func(i, j int) bool { return f.Exemplos[i].Bytes > f.Exemplos[j].Bytes })
		if len(f.Exemplos) > 3 {
			f.Exemplos = f.Exemplos[:3]
		}
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == "outros" != (out[j].ID == "outros") {
			return out[j].ID == "outros"
		}
		return out[i].Bytes > out[j].Bytes
	})
	return out
}
