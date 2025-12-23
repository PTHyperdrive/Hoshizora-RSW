package main

import (
	"encoding/json"
	"fmt"
	"io" // <-- added
	"net"
	"net/http"
	"os"
	// <-- added
)

func (n *Node) serveHTTP() {
	mux := http.NewServeMux()

	mux.HandleFunc("/id", func(w http.ResponseWriter, r *http.Request) {
		type resp struct {
			NodeID string   `json:"nodeId"`
			PeerID string   `json:"peerId"`
			Addrs  []string `json:"addrs"`
			Geo    string   `json:"geo"`
		}
		out := resp{NodeID: n.nodeID, PeerID: n.peerID.String(), Geo: n.geo}
		for _, a := range n.h.Addrs() {
			out.Addrs = append(out.Addrs, fmt.Sprintf("%s/p2p/%s", a, n.peerID))
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/setgeo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct{ GEO string }
		if json.NewDecoder(r.Body).Decode(&req) != nil || trim(req.GEO) == "" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		n.geo = req.GEO
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		type pr struct{ ID, RTT string }
		var list []pr
		n.latMu.Lock()
		for _, p := range n.h.Network().Peers() {
			list = append(list, pr{p.String(), n.rtts[p].String()})
		}
		n.latMu.Unlock()
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("/nearest", func(w http.ResponseWriter, r *http.Request) {
		id, rtt := n.nearestPeer()
		_ = json.NewEncoder(w).Encode(struct{ PeerID, RTT string }{id.String(), rtt.String()})
	})

	mux.HandleFunc("/chat/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct{ Text string }
		if json.NewDecoder(r.Body).Decode(&req) != nil || trim(req.Text) == "" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		if err := n.publishChat(req.Text); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/chat/log", func(w http.ResponseWriter, r *http.Request) {
		n.chatMu.Lock()
		defer n.chatMu.Unlock()
		_ = json.NewEncoder(w).Encode(n.chatLog)
	})

	mux.HandleFunc("/file/send", handleFileSend(n))
	mux.HandleFunc("/file/list", func(w http.ResponseWriter, r *http.Request) {
		n.fileMu.Lock()
		defer n.fileMu.Unlock()
		type item struct {
			ID, FileName string
			Chunks       int
			Complete     bool
		}
		var out []item
		for id, m := range n.manifests {
			comp := len(n.recvMap[id]) == m.Chunks
			out = append(out, item{id, m.FileName, m.Chunks, comp})
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	s := &http.Server{Addr: httpAddr, Handler: logReq(mux)}
	go s.ListenAndServe()
}
func handleFileSend(n *Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(128 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		tmp := tempUploadPath(hdr.Filename)
		out, err := os.Create(tmp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := io.Copy(out, f); err != nil {
			out.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out.Close()
		man, err := n.broadcastFile(tmp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(man)
	}
}

func logReq(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		fmt.Printf("%s %s <- %s\n", r.Method, r.URL.Path, host)
		next.ServeHTTP(w, r)
	})
}
