package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
	"github.com/unicornultrafoundation/subnet-rayai-node/config"
)

// Resources represents system resources
type Resources struct {
	CPUCount       int    `json:"cpu"`
	CPUName        string `json:"cpu_name,omitempty"`
	MemoryTotal    uint64 `json:"memory_total"`
	MemoryFree     uint64 `json:"memory_free"`
	DiskTotal      uint64 `json:"disk_total"`
	DiskFree       uint64 `json:"disk_free"`
	GPUCount       int    `json:"gpu"`
	GPUModel       string `json:"gpu_model,omitempty"`
	GPUMemoryTotal uint64 `json:"gpu_memory_total,omitempty"` // MiB
	GPUMemoryFree  uint64 `json:"gpu_memory_free,omitempty"`  // MiB
}

// Manager handles resource management and node registration
type Manager struct {
	config     *config.Config
	mutex      sync.RWMutex
	resources  *Resources
	lastUpdate time.Time
	updateFreq time.Duration
	client     *http.Client
	registered bool // Add explicit registration status flag

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
func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		config:     cfg,
		updateFreq: time.Minute * 5, // Update resource data every 5 minutes
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		speedTest: speedtest.New(),
	}

	// Initial resource update
	_, _ = m.GetResources(true)

	return m
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

// SendHeartbeat sends a heartbeat to the manager with all node information
func (m *Manager) SendHeartbeat() error {
	if m.config.ManagerIP == "" {
		return fmt.Errorf("manager IP not configured")
	}

	if !m.registered {
		// Not registered yet, try registering first
		if err := m.RegisterNode(); err != nil {
			return fmt.Errorf("cannot send heartbeat, node not registered: %w", err)
		}
	}

	// Send heartbeat to manager
	url := fmt.Sprintf("http://%s/api/heartbeat", m.config.ManagerIP)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	defer resp.Body.Close()

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

// GetResources returns node resource information (CPU, RAM, Disk, GPU)
func (m *Manager) GetResources(forceUpdate bool) (*Resources, error) {
	m.mutex.RLock()
	if !forceUpdate && m.resources != nil && time.Since(m.lastUpdate) < m.updateFreq {
		defer m.mutex.RUnlock()
		return m.resources, nil
	}
	m.mutex.RUnlock()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Double-check after acquiring the write lock
	if !forceUpdate && m.resources != nil && time.Since(m.lastUpdate) < m.updateFreq {
		return m.resources, nil
	}

	// CPU
	cpuCount := runtime.NumCPU()

	// Memory
	var memStats syscall.Sysinfo_t
	memTotal := uint64(0)
	memFree := uint64(0)
	if err := syscall.Sysinfo(&memStats); err == nil {
		memTotal = memStats.Totalram * uint64(memStats.Unit)
		memFree = memStats.Freeram * uint64(memStats.Unit)
	}

	// Disk
	var stat syscall.Statfs_t
	diskTotal := uint64(0)
	diskFree := uint64(0)
	if err := syscall.Statfs("/data", &stat); err == nil {
		diskTotal = stat.Blocks * uint64(stat.Bsize)
		diskFree = stat.Bfree * uint64(stat.Bsize)
	}

	// GPU (try nvidia-smi, fallback to 0)
	gpuCount := 0
	gpuModel := ""
	var gpuMemTotal uint64 = 0
	var gpuMemFree uint64 = 0

	// Use nvidia-smi to query GPU name, total memory, and free memory (in MiB)
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,memory.free", "--format=csv,noheader,nounits")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
		gpuCount = len(lines)
		if gpuCount > 0 {
			// Only extract info from the first GPU (if multiple GPUs exist)
			parts := bytes.Split(lines[0], []byte(","))
			if len(parts) >= 3 {
				gpuModel = string(bytes.TrimSpace(parts[0]))
				// memory.total and memory.free are in MiB (no "MiB" suffix due to nounits)
				if mt, err := parseUint64(bytes.TrimSpace(parts[1])); err == nil {
					gpuMemTotal = mt
				}
				if mf, err := parseUint64(bytes.TrimSpace(parts[2])); err == nil {
					gpuMemFree = mf
				}
			}
		}
	}

	// Get CPU name (Linux only)
	cpuName := ""
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			if bytes.HasPrefix(line, []byte("model name")) {
				parts := bytes.SplitN(line, []byte(":"), 2)
				if len(parts) == 2 {
					cpuName = strings.TrimSpace(string(parts[1]))
					break
				}
			}
		}
	}

	m.resources = &Resources{
		CPUCount:       cpuCount,
		CPUName:        cpuName,
		MemoryTotal:    memTotal,
		MemoryFree:     memFree,
		DiskTotal:      diskTotal,
		DiskFree:       diskFree,
		GPUCount:       gpuCount,
		GPUModel:       gpuModel,
		GPUMemoryTotal: gpuMemTotal,
		GPUMemoryFree:  gpuMemFree,
	}
	m.lastUpdate = time.Now()

	return m.resources, nil
}

// parseUint64 parses a byte slice to uint64
func parseUint64(b []byte) (uint64, error) {
	return strconv.ParseUint(string(b), 10, 64)
}

// RegisterNode registers the node with the manager
func (m *Manager) RegisterNode() error {
	if m.config.ManagerIP == "" {
		return fmt.Errorf("manager IP not configured")
	}

	log.Printf("Registering with manager at %s", m.config.ManagerIP)

	// Get node information
	hostname, _ := os.Hostname()

	// Get complete system information
	resources, err := m.GetResources(true)
	if err != nil {
		return fmt.Errorf("failed to get system resources: %w", err)
	}

	// Get geo location
	geo, err := m.GetGeoLocation()
	if err != nil {
		log.Printf("Warning: Could not get geo location: %v", err)
		// Continue without geo info
	}

	// Get bandwidth information (don't force update to avoid delays)
	bw, err := m.GetBandwidth(false)
	if err != nil {
		log.Printf("Warning: Could not get bandwidth info: %v", err)
		// Continue without bandwidth info
	}

	// Prepare complete registration data
	regData := map[string]interface{}{
		"hostname":  hostname,
		"resources": resources,
		"timestamp": time.Now().Unix(),
	}

	// Add optional data if available
	if geo != nil {
		regData["geo"] = geo
	}
	if bw != nil {
		regData["bandwidth"] = bw
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(regData)
	if err != nil {
		return fmt.Errorf("failed to marshal registration data: %w", err)
	}

	// Send registration to manager
	url := fmt.Sprintf("http://%s/api/register", m.config.ManagerIP)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to manager: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status: %s", resp.Status)
	}

	// Parse response to get node ID
	var result struct {
		NodeID string `json:"node_id"`
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse registration response: %w", err)
	}

	m.registered = true // Still mark as registered even without nodeID
	log.Printf("Registered successfully")
	return nil
}

// IsRegistered returns whether the node has registered with a manager
func (m *Manager) IsRegistered() bool {
	return m.registered
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
