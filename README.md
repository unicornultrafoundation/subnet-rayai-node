# Subnet RayAI Node

**Subnet RayAI Node** is a lightweight decentralized node for distributed AI workloads using [Ray](https://www.ray.io/) over a custom subnet overlay network. Each node can act as a Ray head or worker container, with built-in support for verifier integration and resource usage reporting.

---

## 🚀 Features

* **Ray Cluster Integration**
  Launch Ray Head or Worker nodes dynamically using Docker or containerd.

* **Subnet Overlay Networking**
  Connect nodes over a distributed subnet without centralized control.

* **Verifier Support**
  Integrate verifier modules to check the correctness of AI task execution.

* **Resource Accounting**
  Collect usage stats (CPU, RAM, bandwidth) for on-chain/off-chain reporting.

* **API Server**
  Exposes HTTP endpoints to start Ray head or worker nodes programmatically.

---

## ⚙️ Requirements

* Go 1.21+
* Docker or containerd
* Linux (preferred), MacOS, or Windows (with adjustments)

---

## 🔧 Usage

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| API_PORT | Port for API server | 8080 |
| RAY_BIN_PATH | Path to Ray binary | ray (from PATH) |
| ALLOWED_IPS | Comma-separated list of allowed IPs/CIDR | 127.0.0.1 |
| LOG_LEVEL | Logging level | info |

### Run API Server

```bash
go run cmd/rayai-node/main.go  # API server will start on port 3333 by default
```

### Call API to Start Ray Head

```http
POST http://localhost:3333/start/head
Content-Type: application/json

{
  "port": 6379,
  "username": "rayuser",
  "password": "raypass"
}
```

### Call API to Start Ray Worker

```http
POST /start/worker
Content-Type: application/json

{
  "headIP": "192.168.1.10",
  "username": "rayuser",
  "password": "raypass"
}
```

---

## 🐳 Docker Deployment

### Build Docker Image

```bash
docker build -t rayai-node .
```

### Run Container

```bash
docker run -d \
  --name rayai-node \
  -p 3333:3333 \
  -p 6379:6379 \
  -p 8265:8265 \
  -e ALLOWED_IPS=* \
  rayai-node
```

### Using Docker Compose

```bash
docker-compose up -d
```

---

## 🔐 Security

* Optional verifier nodes cross-check outputs and behaviors.
* Nodes may sign reports and challenge results for slashing protection.

---

## 📚 License

MIT © 2025 Unicorn Ultra Foundation
