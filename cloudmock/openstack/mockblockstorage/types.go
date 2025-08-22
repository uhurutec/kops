/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mockblockstorage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

type VolumeType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type volumeTypeListResponse struct {
	VolumeTypes []VolumeType `json:"volume_types"`
}

type volumeTypeGetResponse struct {
	VolumeType VolumeType `json:"volume_type"`
}

// mockTypes registers handlers for Cinder volume types API.
func (m *MockClient) mockTypes() {
	re := regexp.MustCompile(`/types/?`)

	// Seed a default volume type (id "standard").
	if _, exists := m.volumeTypes["standard"]; !exists {
		m.volumeTypes["standard"] = VolumeType{
			ID:   "standard",
			Name: "standard",
		}
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		w.Header().Add("Content-Type", "application/json")
		typeID := re.ReplaceAllString(r.URL.Path, "")

		switch r.Method {
		case http.MethodGet:
			if typeID == "" {
				m.listTypes(w)
			} else {
				m.getType(w, typeID)
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}

	m.Mux.HandleFunc("/types/", handler)
	m.Mux.HandleFunc("/types", handler)
}

func (m *MockClient) listTypes(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)

	list := make([]VolumeType, 0, len(m.volumeTypes))
	for _, t := range m.volumeTypes {
		list = append(list, t)
	}

	resp := volumeTypeListResponse{VolumeTypes: list}
	respB, err := json.Marshal(resp)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %+v", resp))
	}
	if _, err := w.Write(respB); err != nil {
		panic("failed to write body")
	}
}

func (m *MockClient) getType(w http.ResponseWriter, typeID string) {
	if t, ok := m.volumeTypes[typeID]; ok {
		resp := volumeTypeGetResponse{VolumeType: t}
		respB, err := json.Marshal(resp)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal %+v", resp))
		}
		if _, err := w.Write(respB); err != nil {
			panic("failed to write body")
		}
		return
	}
	w.WriteHeader(http.StatusNotFound)
}
