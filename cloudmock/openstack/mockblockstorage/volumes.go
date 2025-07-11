/*
Copyright 2020 The Kubernetes Authors.

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
	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type JSONRFC3339MilliNoZ gophercloud.JSONRFC3339MilliNoZ

const RFC3339NoZ = "2006-01-02T15:04:05"

func (l *JSONRFC3339MilliNoZ) MarshalJSON() ([]byte, error) {
	t := time.Time(*l)
	s := `"` + t.Format(RFC3339NoZ) + `"`
	return []byte(s), nil
}

type ExtendedVolumeType struct {
	volumes.Volume
	CreatedAt JSONRFC3339MilliNoZ `json:"created_at"`
	UpdatedAt JSONRFC3339MilliNoZ `json:"updated_at"`
}

type volumeListResponse struct {
	Volumes []ExtendedVolumeType `json:"volumes"`
}

type volumeGetResponse struct {
	Volume ExtendedVolumeType `json:"volume"`
}

type volumeCreateRequest struct {
	Volume volumes.CreateOpts `json:"volume"`
}

type volumeUpdateRequest struct {
	Volume volumes.UpdateOpts `json:"volume"`
}

func (m *MockClient) mockVolumes(extraMocks ExtraMocks) {
	re := regexp.MustCompile(`/volumes/?`)
	updateCounter := 0

	handler := func(w http.ResponseWriter, r *http.Request) {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		w.Header().Add("Content-Type", "application/json")
		volID := re.ReplaceAllString(r.URL.Path, "")
		switch r.Method {
		case http.MethodGet:
			if volID == "detail" {
				r.ParseForm()
				m.listVolumes(w, r.Form)
			} else {
				m.getVolume(w, volID)
			}
		case http.MethodPost:
			m.createVolume(w, r, extraMocks.Create)
		case http.MethodPut:
			m.updateVolume(w, r, volID, extraMocks.Update[updateCounter])
			updateCounter++
		case http.MethodDelete:
			m.deleteVolume(w, volID)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}
	m.Mux.HandleFunc("/volumes/", handler)
	m.Mux.HandleFunc("/volumes", handler)
}

func MarshalVolume(volume volumes.Volume) ([]byte, error) {
	var res []byte
	var newVolume volumeGetResponse

	newVol, err := AddMocksReplaceVolumes(&volume)
	if err != nil {
		return nil, err
	}

	newVolume.Volume = newVol

	res, err = json.Marshal(&newVolume)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func MarshalVolumes(volumes []volumes.Volume) ([]byte, error) {
	var res []byte
	var newVolumes volumeListResponse

	for _, v := range volumes {
		newVolume, err := AddMocksReplaceVolumes(&v)
		if err != nil {
			return nil, err
		}
		newVolumes.Volumes = append(newVolumes.Volumes, newVolume)
	}

	res, err := json.Marshal(newVolumes)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func AddMocksReplaceVolumes(r *volumes.Volume) (ExtendedVolumeType, error) {

	newVol := ExtendedVolumeType{
		*r,
		JSONRFC3339MilliNoZ(r.CreatedAt),
		JSONRFC3339MilliNoZ(r.UpdatedAt),
	}
	return newVol, nil
}

func (m *MockClient) listVolumes(w http.ResponseWriter, vals url.Values) {
	w.WriteHeader(http.StatusOK)

	vols := filterVolumes(m.volumes, vals)

	respB, err := MarshalVolumes(vols)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %+v", vols))
	}
	_, err = w.Write(respB)
	if err != nil {
		panic("failed to write body")
	}
}

func (m *MockClient) getVolume(w http.ResponseWriter, volumeID string) {
	if vol, ok := m.volumes[volumeID]; ok {
		respB, err := MarshalVolume(vol)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal %+v", vol))
		}
		_, err = w.Write(respB)
		if err != nil {
			panic("failed to write body")
		}
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *MockClient) updateVolume(w http.ResponseWriter, r *http.Request, volumeID string, mocks UpdateMocks) {
	if _, ok := m.volumes[volumeID]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var update volumeUpdateRequest
	err := json.NewDecoder(r.Body).Decode(&update)
	if err != nil {
		panic("error decoding update volume request")
	}
	vol := m.volumes[volumeID]
	vol.Metadata = update.Volume.Metadata
	vol.UpdatedAt = mocks.UpdatedAt
	vol.VolumeType = mocks.VolumeType
	vol.Size = mocks.Size
	m.volumes[volumeID] = vol

	w.WriteHeader(http.StatusOK)

	respB, err := MarshalVolume(vol)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %+v", vol))
	}
	_, err = w.Write(respB)
	if err != nil {
		panic("failed to write body")
	}
}

func (m *MockClient) deleteVolume(w http.ResponseWriter, volumeID string) {
	if _, ok := m.volumes[volumeID]; ok {
		delete(m.volumes, volumeID)
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *MockClient) createVolume(w http.ResponseWriter, r *http.Request, mocks CreateMocks) {
	var create volumeCreateRequest
	err := json.NewDecoder(r.Body).Decode(&create)
	if err != nil {
		panic("error decoding create volume request")
	}

	w.WriteHeader(http.StatusAccepted)

	v := volumes.Volume{
		ID:               uuid.New().String(),
		Name:             create.Volume.Name,
		Size:             create.Volume.Size,
		AvailabilityZone: create.Volume.AvailabilityZone,
		Metadata:         create.Volume.Metadata,
		VolumeType:       create.Volume.VolumeType,
		CreatedAt:        mocks.CreatedAt,
		UpdatedAt:        mocks.UpdatedAt,
	}
	m.volumes[v.ID] = v

	respB, err := MarshalVolume(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %+v", v))
	}
	_, err = w.Write(respB)
	if err != nil {
		panic("failed to write body")
	}
}

func filterVolumes(allVolumes map[string]volumes.Volume, vals url.Values) []volumes.Volume {
	vols := make([]volumes.Volume, 0)
	for _, volume := range allVolumes {
		name := vals.Get("name")
		metadata := vals.Get("metadata")
		// metadata is decoded as: {'k8s.io/etcd/main':'1/1',+'k8s.io/role/master':'1'}
		// Replacing single quotes with double quotes makes it valid JSON
		metadata = strings.ReplaceAll(metadata, "'", "\"")
		parsedMetadata := make(map[string]string)
		json.Unmarshal([]byte(metadata), &parsedMetadata)

		if name != "" && volume.Name != name {
			continue
		}
		match := true
		for k, v := range parsedMetadata {
			if volume.Metadata[k] != v {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		vols = append(vols, volume)
	}
	return vols
}
