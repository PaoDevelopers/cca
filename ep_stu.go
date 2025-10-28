package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (app *App) handleStu(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Alloewd", http.StatusMethodNotAllowed)
		return
	}

	fmt.Fprint(w, `Hi! You have logged on as a student (see info below) but there's
no student UI yet (poke Henry!). In the future there will be a JS SPA
over here. Note that all auth-related things are already done; you can
use the network inspector to check cookie status.

`)

	json.NewEncoder(w).Encode(sui)
}
