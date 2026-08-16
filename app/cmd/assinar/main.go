//go:build windows

// Comando de PUBLICACAO: gera o par de chaves e assina o release.
//
// Vive fora do binario do produto de proposito. O ThazzDraco.exe embute um
// manifesto requireAdministrator — se a assinatura rodasse por ele, o passo de
// build pediria elevacao sem motivo nenhum. Aqui e um binario comum.
//
// Uso:
//
//	go run ./cmd/assinar -gerar-chaves E:\caminho\fora\do\repo\update.key
//	go run ./cmd/assinar -assinar dist\ThazzDraco.exe -chave E:\...\update.key
//
// A chave privada NUNCA entra no repositorio: quem a tiver assina atualizacoes
// que todo cliente instala como administrador.
package main

import (
	"flag"
	"fmt"
	"os"

	"thazzdraco/internal/winutil"
)

func main() {
	gerar := flag.String("gerar-chaves", "", "gera o par e grava a chave privada neste caminho")
	assinar := flag.String("assinar", "", "arquivo a assinar")
	chave := flag.String("chave", "", "caminho da chave privada")
	flag.Parse()

	switch {
	case *gerar != "":
		pub, err := winutil.GerarParDeChaves(*gerar)
		if err != nil {
			morre(err)
		}
		fmt.Println("chave privada gravada em:", *gerar)
		fmt.Println("GUARDE FORA DO REPOSITORIO E FACA BACKUP — sem ela, nao ha como publicar update.")
		fmt.Println()
		fmt.Println("chave publica (embutir em winutil.ChavePublicaAtualizacao):")
		fmt.Println(pub)

	case *assinar != "":
		if *chave == "" {
			morre(fmt.Errorf("informe -chave com o caminho da chave privada"))
		}
		sig, err := winutil.AssinarArquivo(*assinar, *chave)
		if err != nil {
			morre(err)
		}
		d, _ := winutil.DigestArquivo(*assinar)
		fmt.Printf("assinado: %s\nsha256:   %x\n", sig, d)

	default:
		flag.Usage()
		os.Exit(2)
	}
}

func morre(err error) {
	fmt.Fprintln(os.Stderr, "erro:", err)
	os.Exit(1)
}
