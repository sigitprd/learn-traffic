package main

import (
	"net/http"
	"os"
)

// crashHandler sengaja mematikan proses TOTAL (os.Exit, bukan panic yang
// bisa di-recover http server-nya) - buat eksperimen Level 6 Experiment C:
// bedain "app crash dari dalam" vs "kubectl delete pod" (Level 3) vs
// "OOMKilled" (Level 4). HANYA dipanggil lewat port-forward ke Pod
// spesifik, JANGAN lewat Service/Ingress.
func crashHandler(w http.ResponseWriter, r *http.Request) {
	if appLog != nil {
		appLog.Printf("GET /crash dipanggil dari %s - proses akan exit(1) sekarang", r.RemoteAddr)
	}
	os.Exit(1)
}
