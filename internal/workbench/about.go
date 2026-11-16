package workbench

import (
	"net/http"

	"github.com/sylvinus/agent-vm/internal/version"
)

func (s *Server) appAbout(w http.ResponseWriter, _ *http.Request) {
	jsonReply(w, version.CurrentBuild())
}
