package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
	"github.com/unicornultrafoundation/subnet-rayai-node/config"
	"github.com/unicornultrafoundation/subnet-rayai-node/ray"
)

// Manager handles resource management and node registration
type Manager struct {
	config     *config.Config
	rayService *ray.Service
	nodeID     string
	client     *http.Client

	// Geo location cache
	geoMutex  sync.RWMutex
	geoCache  *GeoLocation
	geoExpiry time.Time

	// Bandwidth cache
	bwMutex   sync.RWMutex
	bwCache   *Bandwidth
	bwExpiry  time.Time
	speedTest *speedtest.Speedtest
}

// GeoLocation stores node geographical information
type GeoLocation struct {
	IP        string  `json:"ip"`
	City      string  `json:"city"`
	Region    string  `json:"region"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Bandwidth stores node network bandwidth information
type Bandwidth struct {
	DownloadMbps float64 `json:"download_mbps"`
	UploadMbps   float64 `json:"upload_mbps"`
	PingMs       int64   `json:"ping_ms"`
	Jitter       int64   `json:"jitter"`
	MeasuredAt   int64   `json:"measured_at"`
}

// NewManager creates a new resource manager
func NewManager(cfg *config.Config, rayService *ray.Service) *Manager {
	return &Manager{
		config:     cfg,
		rayService: rayService,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		speedTest: speedtest.New(),
	}
}

// StartHeartbeat begins sending periodic heartbeats to the manager
func (m *Manager) StartHeartbeat(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			<-ticker.C
			if err := m.SendHeartbeat(); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
			}
		}
	}()
}

// SendHeartbeat sends a heartbeat to the manager
func (m *Manager) SendHeartbeat() error {
	// Get latest resource usage
	resources, err := m.rayService.GetResources()
	if err != nil {
		return fmt.Errorf("failed to get resources for heartbeat: %w", err)
	}

	// Get geo location
	geo, _ := m.GetGeoLocation()

	// Get bandwidth (only if available, don't measure now)
	bw, _ := m.GetBandwidth(false)

	heartbeatData := map[string]interface{}{
		"node_id":   m.nodeID,
		"resources": resources,
		"timestamp": time.Now().Unix(),
	}

	// Add geo and bandwidth if available
	if geo != nil {
		heartbeatData["geo"] = geo
	}

	if bw != nil {
		heartbeatData["bandwidth"] = bw
	}

	heartbeatJSON, err := json.Marshal(heartbeatData)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat data: %w", err)
	}

	// Prepare the request
	url := fmt.Sprintf("http://%s/api/heartbeat", m.config.ManagerIP)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(heartbeatJSON))
	if err != nil {
		return fmt.Errorf("failed to create heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed with status: %s", resp.Status)
	}

	return nil
}

// GetGeoLocation returns the node's geographical location
func (m *Manager) GetGeoLocation() (*GeoLocation, error) {
	m.geoMutex.RLock()
	if m.geoCache != nil && time.Now().Before(m.geoExpiry) {
		defer m.geoMutex.RUnlock()
		return m.geoCache, nil
	}
	m.geoMutex.RUnlock()

	// Need to refresh the cache
	m.geoMutex.Lock()
	defer m.geoMutex.Unlock()

	// Check again after acquiring write lock
	if m.geoCache != nil && time.Now().Before(m.geoExpiry) {
		return m.geoCache, nil
	}

	// Make request to ip-api.com
	resp, err := m.client.Get("http://ip-api.com/json/")
	if err != nil {
		return nil, fmt.Errorf("error getting geo location: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading geo response: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing geo response: %v", err)
	}

	geo := &GeoLocation{
		IP:      fmt.Sprintf("%v", result["query"]),
		City:    fmt.Sprintf("%v", result["city"]),
		Region:  fmt.Sprintf("%v", result["regionName"]),
		Country: fmt.Sprintf("%v", result["country"]),
	}

	if lat, ok := result["lat"].(float64); ok {
		geo.Latitude = lat
	}
	if lon, ok := result["lon"].(float64); ok {
		geo.Longitude = lon
	}

	// Cache for 4 hours
	m.geoCache = geo
	m.geoExpiry = time.Now().Add(4 * time.Hour)

	return geo, nil
}

// GetBandwidth returns the node's network bandwidth
func (m *Manager) GetBandwidth(forceUpdate bool) (*Bandwidth, error) {
	m.bwMutex.RLock()
	if !forceUpdate && m.bwCache != nil && time.Now().Before(m.bwExpiry) {
		defer m.bwMutex.RUnlock()
		return m.bwCache, nil
	}
	m.bwMutex.RUnlock()

	// Need to refresh the cache
	m.bwMutex.Lock()
	defer m.bwMutex.Unlock()

	// Check again after acquiring write lock
	if !forceUpdate && m.bwCache != nil && time.Now().Before(m.bwExpiry) {
		return m.bwCache, nil
	}

	serverList, err := m.speedTest.FetchServers()
	if err != nil {
		return nil, fmt.Errorf("error fetching speedtest servers: %v", err)
	}

	if len(serverList) < 1 {
		return nil, fmt.Errorf("no speedtest servers found")
	}

	targets := serverList[:1] // Use the closest server
	for _, server := range targets {
		server.PingTest(func(latency time.Duration) {
			// Optionally handle latency here, e.g., log or assign to server.Latency
			server.Latency = latency
		})
		server.DownloadTest()
		server.UploadTest()

		bw := &Bandwidth{
			DownloadMbps: server.DLSpeed.Mbps(),
			UploadMbps:   server.ULSpeed.Mbps(),
			PingMs:       server.Latency.Milliseconds(),
			Jitter:       server.Jitter.Milliseconds(),
			MeasuredAt:   time.Now().Unix(),
		}

		// Cache for 24 hours
		m.bwCache = bw
		m.bwExpiry = time.Now().Add(24 * time.Hour)

		return bw, nil
	}

	return nil, fmt.Errorf("bandwidth test failed to complete")
}

// StartBackgroundUpdater starts periodic updates of resource-related data
func (m *Manager) StartBackgroundUpdater() {
	// Periodically update geo location every 4 hours, retry after 1m if error
	go func() {
		for {
			if _, err := m.GetGeoLocation(); err != nil {
				fmt.Printf("Error fetching geo location: %v\n", err)
				time.Sleep(time.Minute)
				continue
			}
			time.Sleep(4 * time.Hour)
		}
	}()

	// Periodically update bandwidth every 24 hours, retry after 1m if error
	go func() {
		for {
			if _, err := m.GetBandwidth(true); err != nil {
				fmt.Printf("Error fetching bandwidth: %v\n", err)
				time.Sleep(time.Minute)
				continue
			}
			time.Sleep(24 * time.Hour)
		}
	}()
}
