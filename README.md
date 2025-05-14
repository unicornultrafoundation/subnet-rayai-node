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

### Run API Server

```bash
go run main.go
```

### Call API to Start Ray Head

```http
POST /start/head
Content-Type: application/json

{
  "port": 6379,
  "shmSize": "8gb",
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
  "shmSize": "8gb",
  "username": "rayuser",
  "password": "raypass"
}
```

---

## 🔐 Security

* Optional verifier nodes cross-check outputs and behaviors.
* Nodes may sign reports and challenge results for slashing protection.

---

## 📚 License

MIT © 2025 Unicorn Ultra Foundation
