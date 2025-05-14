package mockcompute

import (
	"encoding/json"
	"net/http"
)

type instanceActionsResponse struct {
	InstanceActions []interface{} `json:"instanceActions"`
}

func (m *MockClient) mockInstanceActions() {
	//re := regexp.MustCompile(`/servers/(.*?)/os-instance-actions/?`)

	handler := func(w http.ResponseWriter, r *http.Request) {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		w.Header().Add("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		resp := instanceActionsResponse{
			InstanceActions: make([]interface{}, 0),
		}
		respB, err := json.Marshal(resp)
		if err != nil {
			panic("failed to marshal response")
		}
		_, err = w.Write(respB)
		if err != nil {
			panic("failed to write body")
		}
	}
	m.Mux.HandleFunc("/servers/{server_id}/os-instance-actions/", handler)
}
