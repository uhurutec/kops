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

package mockcompute

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"k8s.io/kops/upup/pkg/fi"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

type JSONRFC3339MilliNoZ gophercloud.JSONRFC3339MilliNoZ

const RFC3339NoZ = "2006-01-02T15:04:05"

func (l *JSONRFC3339MilliNoZ) MarshalJSON() ([]byte, error) {
	t := time.Time(*l)
	s := `"` + t.Format(RFC3339NoZ) + `"`
	return []byte(s), nil
}

type ExtendedServerType struct {
	servers.Server
	Created    JSONRFC3339MilliNoZ `json:"-"`
	Updated    JSONRFC3339MilliNoZ `json:"-"`
	LaunchedAt JSONRFC3339MilliNoZ `json:"OS-SRV-USG:launched_at"`
}

type serverGetResponse struct {
	Server ExtendedServerType `json:"server"`
}

type serverListResponse struct {
	Servers []ExtendedServerType `json:"servers"`
}

type serverCreateRequest struct {
	Server Server `json:"server"`
}

type serverUpdateRequest struct {
	Server servers.UpdateOpts `json:"server"`
}

// CreateOpts specifies server creation parameters.
type Server struct {
	// Name is the name to assign to the newly launched server.
	Name string `json:"name" required:"true"`

	// ImageRef [optional; required if ImageName is not provided] is the ID or
	// full URL to the image that contains the server's OS and initial state.
	// Also optional if using the boot-from-volume extension.
	ImageRef string `json:"imageRef"`

	// ImageName [optional; required if ImageRef is not provided] is the name of
	// the image that contains the server's OS and initial state.
	// Also optional if using the boot-from-volume extension.
	ImageName string `json:"-"`

	// FlavorRef [optional; required if FlavorName is not provided] is the ID or
	// full URL to the flavor that describes the server's specs.
	FlavorRef string `json:"flavorRef"`

	// FlavorName [optional; required if FlavorRef is not provided] is the name of
	// the flavor that describes the server's specs.
	FlavorName string `json:"-"`

	// SecurityGroups lists the names of the security groups to which this server
	// should belong.
	SecurityGroups []string `json:"-"`

	// UserData contains configuration information or scripts to use upon launch.
	// Create will base64-encode it for you, if it isn't already.
	UserData []byte `json:"-"`

	// AvailabilityZone in which to launch the server.
	AvailabilityZone string `json:"availability_zone,omitempty"`

	// Networks dictates how this server will be attached to available networks.
	// By default, the server will be attached to all isolated networks for the
	// tenant.
	// Starting with microversion 2.37 networks can also be an "auto" or "none"
	// string.
	Networks []Networks `json:"networks"`

	// Metadata contains key-value pairs (up to 255 bytes each) to attach to the
	// server.
	Metadata map[string]string `json:"metadata,omitempty"`

	// ConfigDrive enables metadata injection through a configuration drive.
	ConfigDrive *bool `json:"config_drive,omitempty"`

	// AdminPass sets the root user password. If not set, a randomly-generated
	// password will be created and returned in the response.
	AdminPass string `json:"adminPass,omitempty"`

	// AccessIPv4 specifies an IPv4 address for the instance.
	AccessIPv4 string `json:"accessIPv4,omitempty"`

	// AccessIPv6 specifies an IPv6 address for the instance.
	AccessIPv6 string `json:"accessIPv6,omitempty"`

	// Min specifies Minimum number of servers to launch.
	Min int `json:"min_count,omitempty"`

	// Max specifies Maximum number of servers to launch.
	Max int `json:"max_count,omitempty"`

	// ServiceClient will allow calls to be made to retrieve an image or
	// flavor ID by name.
	ServiceClient *gophercloud.ServiceClient `json:"-"`

	// Tags allows a server to be tagged with single-word metadata.
	// Requires microversion 2.52 or later.
	Tags []string `json:"tags,omitempty"`
}

type Networks struct {
	Port string `json:"port,omitempty"`
}

func (m *MockClient) mockServers(extraMocks ExtraMocks) {
	re := regexp.MustCompile(`/servers/?`)
	updateCounter := 0

	handler := func(w http.ResponseWriter, r *http.Request) {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		w.Header().Add("Content-Type", "application/json")

		serverID := re.ReplaceAllString(r.URL.Path, "")
		switch r.Method {
		case http.MethodGet:
			if serverID == "detail" {
				r.ParseForm()
				m.listServers(w, r.Form)
			}
			m.getServer(w, serverID)
		case http.MethodPost:
			m.createServer(w, r, extraMocks.Create)
		case http.MethodPut:
			m.updateServer(w, r, serverID, extraMocks.Update[updateCounter])
			updateCounter++
		case http.MethodDelete:
			m.deleteServer(w, serverID)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}
	m.Mux.HandleFunc("/servers/", handler)
	m.Mux.HandleFunc("/servers", handler)
}

func MarshalServer(server servers.Server) ([]byte, error) {
	var res []byte
	var newServer serverGetResponse

	newSrv, err := AddMocksReplaceServers(&server)
	if err != nil {
		return nil, err
	}

	newServer.Server = newSrv

	res, err = json.Marshal(&newServer)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func MarshalServers(servers []servers.Server) ([]byte, error) {
	var res []byte
	var newServers serverListResponse

	for _, v := range servers {
		newServer, err := AddMocksReplaceServers(&v)
		if err != nil {
			return nil, err
		}
		newServers.Servers = append(newServers.Servers, newServer)
	}

	res, err := json.Marshal(newServers)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func AddMocksReplaceServers(r *servers.Server) (ExtendedServerType, error) {

	newSrv := ExtendedServerType{
		*r,
		JSONRFC3339MilliNoZ(r.Created),
		JSONRFC3339MilliNoZ(r.Updated),
		JSONRFC3339MilliNoZ(r.LaunchedAt),
	}
	return newSrv, nil
}

func (m *MockClient) getServer(w http.ResponseWriter, serverID string) {
	if server, ok := m.servers[serverID]; ok {
		resp8, err := MarshalServer(server)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal %+v", server))
		}
		_, err = w.Write(resp8)
		if err != nil {
			panic("failed to write body")
		}
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *MockClient) listServers(w http.ResponseWriter, vals url.Values) {
	serverName := strings.Trim(vals.Get("name"), "^$")
	matched := make([]servers.Server, 0)
	for _, server := range m.servers {
		if strings.HasPrefix(server.Name, serverName) {
			matched = append(matched, server)
		}
	}
	respB, err := MarshalServers(matched)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %+v", matched))
	}
	_, err = w.Write(respB)
	if err != nil {
		panic("failed to write body")
	}
}

func (m *MockClient) updateServer(w http.ResponseWriter, r *http.Request, serverID string, mocks UpdateMocks) {
	if _, ok := m.servers[serverID]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var update serverUpdateRequest
	err := json.NewDecoder(r.Body).Decode(&update)
	if err != nil {
		panic("error decoding update server request")
	}
	server := m.servers[serverID]
	server.Name = update.Server.Name
	server.Updated = mocks.Updated
	server.Flavor["name"] = mocks.Flavor.Name
	server.Flavor["original_name"] = mocks.Flavor.Name
	server.Flavor["ram"] = mocks.Flavor.RAM
	server.Flavor["vcpus"] = mocks.Flavor.VCPUs
	server.Flavor["disk"] = mocks.Flavor.Disk
	server.Flavor["ephemeral"] = mocks.Flavor.Ephemeral

	m.servers[serverID] = server

	w.WriteHeader(http.StatusOK)

	respB, err := MarshalServer(server)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %+v", server))
	}
	_, err = w.Write(respB)
	if err != nil {
		panic("failed to write body")
	}
}

func (m *MockClient) deleteServer(w http.ResponseWriter, serverID string) {
	if _, ok := m.servers[serverID]; ok {
		delete(m.servers, serverID)
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *MockClient) createServer(w http.ResponseWriter, r *http.Request, mocks CreateMocks) {
	var create serverCreateRequest
	err := json.NewDecoder(r.Body).Decode(&create)
	if err != nil {
		panic("error decoding create server request")
	}

	if len(create.Server.Networks) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)

	flavorId := create.Server.FlavorRef
	flavor := m.flavors[flavorId]

	server := servers.Server{
		ID:       uuid.New().String(),
		Name:     create.Server.Name,
		Metadata: create.Server.Metadata,
		Status:   "ACTIVE",
		Flavor: map[string]any{
			"id":            flavor.ID,
			"name":          flavor.Name,
			"original_name": flavor.Name,
			"ram":           flavor.RAM,
			"vcpus":         flavor.VCPUs,
			"disk":          flavor.Disk,
			"ephemeral":     flavor.Ephemeral,
		},
		Created:    mocks.Created,
		Updated:    mocks.Updated,
		LaunchedAt: mocks.LaunchedAt,
	}
	securityGroups := make([]map[string]interface{}, len(create.Server.SecurityGroups))
	for i, groupName := range create.Server.SecurityGroups {
		securityGroups[i] = map[string]interface{}{"name": groupName}
	}
	server.SecurityGroups = securityGroups

	portID := create.Server.Networks[0].Port
	ports.Update(r.Context(), m.networkClient, portID, ports.UpdateOpts{
		DeviceID: fi.PtrTo(server.ID),
	})

	// Assign an IP address
	private := make([]map[string]string, 1)
	private[0] = make(map[string]string)
	private[0]["OS-EXT-IPS:type"] = "fixed"
	private[0]["addr"] = "192.168.1.1"
	server.Addresses = map[string]interface{}{
		"private": private,
	}

	m.servers[server.ID] = server

	respB, err := MarshalServer(server)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %+v", server))
	}
	_, err = w.Write(respB)
	if err != nil {
		panic("failed to write body")
	}
}
