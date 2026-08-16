package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Cookie NAO tem porta no escopo (RFC 6265 §8.5): 127.0.0.1:50999 e
// 127.0.0.1:53218 dividem o mesmo pote de cookies. Enquanto o cookie se chamava
// so "tz_session", a segunda instancia do app sobrescrevia o token da primeira,
// e a janela da primeira passava a receber 403 em TODA chamada — para sempre,
// porque a tela nao recarrega sozinha.
//
// O nome com a porta e o que mantem as duas separadas.

func TestCookieDeSessaoEExclusivoDaPorta(t *testing.T) {
	a := &Server{token: "token-da-primeira", porta: "50999"}
	b := &Server{token: "token-da-segunda", porta: "53218"}

	if a.nomeCookie() == b.nomeCookie() {
		t.Fatalf("duas instâncias usariam o mesmo cookie (%q): a segunda derrubaria a sessão da primeira",
			a.nomeCookie())
	}
	if a.nomeCookie() != sessionCookie+"_50999" {
		t.Fatalf("nome inesperado: %q", a.nomeCookie())
	}
}

func TestSemPortaMantemONomeAntigo(t *testing.T) {
	// Modo headless e testes não definem porta; o nome sem sufixo continua válido.
	s := &Server{token: "x"}
	if s.nomeCookie() != sessionCookie {
		t.Fatalf("sem porta o nome deveria ser %q, veio %q", sessionCookie, s.nomeCookie())
	}
}

// O carimbo e a leitura tem que concordar: se setSessionCookie gravar com um
// nome e tokenOK procurar outro, TODA chamada da janela vira 403.
func TestOQueEhCarimbadoEhOQueEhLido(t *testing.T) {
	s := &Server{token: "segredo-de-sessao", porta: "44444"}

	w := httptest.NewRecorder()
	s.setSessionCookie(w)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("esperava 1 cookie carimbado, veio %d", len(cookies))
	}
	if cookies[0].Name != s.nomeCookie() {
		t.Fatalf("carimbou %q mas tokenOK procura %q", cookies[0].Name, s.nomeCookie())
	}

	r := httptest.NewRequest("GET", "/api/saude", nil)
	r.AddCookie(cookies[0])
	if !s.tokenOK(r) {
		t.Fatal("o cookie que o próprio servidor carimbou não foi aceito por ele")
	}

	// Cookie de OUTRA instância (nome de outra porta) não vale para esta.
	r2 := httptest.NewRequest("GET", "/api/saude", nil)
	r2.AddCookie(&http.Cookie{Name: sessionCookie + "_9999", Value: "segredo-de-sessao"})
	if s.tokenOK(r2) {
		t.Fatal("aceitou o cookie de outra instância")
	}
}
