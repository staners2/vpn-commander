# VPN Commander Bot

[![CI/CD Pipeline](https://github.com/staners2/vpn-commander/actions/workflows/ci-cd.yml/badge.svg)](https://github.com/staners2/vpn-commander/actions/workflows/ci-cd.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/staners2/vpn-commander)](https://goreportcard.com/report/github.com/staners2/vpn-commander)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A Telegram bot for managing VPN routing on Xkeen routers. This bot allows you to switch between direct internet access and VPN routing through a simple Telegram interface.

## Features

- **Secure Authentication**: Users must authenticate with a code to access bot functions
- **VPN Control**: Toggle between direct routing and VPN routing
- **Status Monitoring**: Check current VPN routing status with in-memory caching
- **SSH Integration**: Securely connects to Xkeen router via SSH
- **Configuration Management**: Automatically modifies Xray routing configuration
- **Service Management**: Automatically restarts Xray service after configuration changes
- **Logging**: Comprehensive logging with configurable levels
- **Multi-arch Images**: Docker images for AMD64 and ARM64 architectures
- **Kubernetes Ready**: Helm chart for production deployment
- **GitHub Packages**: Automated publishing to GitHub Container Registry

## Architecture

The bot consists of several components:

- **Telegram Bot**: Handles user interactions and authentication
- **SSH Client**: Manages secure connections to the router
- **VPN Manager**: Handles Xray configuration modifications
- **Main Application**: Orchestrates all components with graceful shutdown

## Requirements

- Go 1.21 or later
- Docker (for containerized deployment)
- Kubernetes cluster (for Kubernetes deployment)
- Helm 3.x (for Kubernetes deployment)
- Xkeen router with SSH access
- Telegram Bot Token

## Installation

### Option 1: Docker Compose (Recommended)

**Quick start with docker-compose:**

1. **Clone and configure**:
   ```bash
   git clone https://github.com/staners2/vpn-commander.git
   cd vpn-commander
   cp .env.example .env
   ```

2. **Edit `.env` file with your configuration**:
   ```bash
   # Required: Get from @BotFather on Telegram
   TELEGRAM_BOT_TOKEN=1234567890:ABCdefGHIjklMNOpqrSTUvwxYZ123456789
   AUTH_CODE=your-secure-auth-code-here
   
   # Router configuration
   ROUTER_HOST=192.168.1.1
   ROUTER_USERNAME=admin
   ROUTER_PASSWORD=your-router-password
   
   # Optional
   LOG_LEVEL=info
   ```

3. **Build and run with docker-compose**:
   ```bash
   # Build and start
   docker-compose up -d --build
   
   # Check status
   docker-compose ps
   docker-compose logs -f vpn-commander
   ```

4. **Manage the service**:
   ```bash
   # Stop
   docker-compose down
   
   # Rebuild and restart (after code changes)
   docker-compose up -d --build
   
   # View health status
   docker-compose exec vpn-commander wget -qO- http://localhost:8080/health
   
   # View logs
   docker-compose logs -f vpn-commander
   ```

### Option 2: Kubernetes with Helm (Production)

**Deploy using the published Helm chart:**

```bash
# Pull the chart
helm pull oci://ghcr.io/staners2/vpn-commander/charts/vpn-commander --untar

# Create values file
cp vpn-commander/values.yaml values.prd.yaml

# Edit values.prd.yaml with your configuration:
# - Set TELEGRAM_BOT_TOKEN from @BotFather
# - Set AUTH_CODE for bot authentication  
# - Set ROUTER_HOST, ROUTER_USERNAME, ROUTER_PASSWORD
# - Adjust other settings as needed

# Install the chart
helm install vpn-commander vpn-commander/ -f values.prd.yaml

# Check deployment
kubectl get pods -l app.kubernetes.io/name=vpn-commander
kubectl logs -f deployment/vpn-commander-vpn-commander
```

### Option 3: Docker Run (Simple)

**Single container deployment:**

```bash
docker run -d \
  --name vpn-commander \
  --restart unless-stopped \
  -e TELEGRAM_BOT_TOKEN="your_bot_token" \
  -e AUTH_CODE="your_auth_code" \
  -e ROUTER_HOST="192.168.1.1" \
  -e ROUTER_USERNAME="admin" \
  -e ROUTER_PASSWORD="your_password" \
  -e LOG_LEVEL="info" \
  ghcr.io/staners2/vpn-commander:latest
```

### Option 4: Local Development

1. **Clone the repository**:
   ```bash
   git clone https://github.com/staners2/vpn-commander.git
   cd vpn-commander
   ```

2. **Install dependencies**:
   ```bash
   go mod tidy
   ```

3. **Configure environment variables**:
   ```bash
   export TELEGRAM_BOT_TOKEN="your_bot_token"
   export AUTH_CODE="your_auth_code"
   export ROUTER_HOST="192.168.1.1"
   export ROUTER_USERNAME="admin"
   export ROUTER_PASSWORD="your_password"
   ```

4. **Build and run**:
   ```bash
   go build -o vpn-commander .
   ./vpn-commander
   ```

## Configuration

### Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `TELEGRAM_BOT_TOKEN` | Telegram bot token from BotFather | Yes | - |
| `AUTH_CODE` | Authentication code for bot access | Yes | - |
| `ROUTER_HOST` | Router IP address or hostname | Yes | - |
| `ROUTER_USERNAME` | SSH username for router | Yes | - |
| `ROUTER_PASSWORD` | SSH password for router | Yes | - |
| `XRAY_CONFIG_PATH` | Path to Xray routing config | No | `/opt/etc/xray/configs/05_routing.json` |
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | No | `info` |

### Xray Configuration Format

The bot expects the Xray routing configuration to have the following structure:

```json
{
  "routing": {
    "domainStrategy": "IPIfNonMatch",
    "rules": [
      {
        "type": "field",
        "inboundTag": ["redirect", "tproxy"],
        "network": "tcp,udp",
        "outboundTag": "direct"
      }
    ]
  }
}
```

The bot will modify the `outboundTag` field:
- `"direct"` for direct routing (VPN disabled)
- `"vless-reality"` for VPN routing (VPN enabled)

## Usage

### Setting up the Telegram Bot

1. **Create a bot**: Message @BotFather on Telegram and create a new bot
2. **Get the token**: Save the bot token from BotFather
3. **Configure the bot**: Set the token in your environment configuration

### Using the Bot

1. **Start the bot**: Send `/start` to get welcome message
2. **Authenticate**: Send `/auth YOUR_AUTH_CODE` to authenticate
3. **Use controls**: Use the keyboard buttons to control VPN:
   - 📊 **Status**: Check current VPN status
   - 🔒 **Enable VPN**: Route traffic through VPN
   - 🌐 **Disable VPN**: Route traffic directly

### Security Considerations

- **Authentication Required**: All users must authenticate with the configured auth code
- **SSH Security**: Uses SSH for secure router communication
- **Environment Variables**: Sensitive data stored in environment variables
- **Container Security**: Runs as non-root user in container
- **Network Policies**: Kubernetes deployment includes network policies

## Development

### Project Structure

```
vpn-commander/
├── main.go              # Main application entry point
├── telegram_bot.go      # Telegram bot implementation
├── ssh_client.go        # SSH client for router communication
├── vpn_manager.go       # VPN configuration management
├── go.mod               # Go module definition
├── go.sum               # Go module checksums
├── Dockerfile           # Container image definition
├── .env.example         # Environment variables template
├── charts/              # Helm chart for Kubernetes
│   └── vpn-commander/
└── .github/             # GitHub Actions workflows
    └── workflows/
        └── ci.yml
```

### Building

```bash
# Build for current platform
go build -o vpn-commander .

# Build for Linux (for Docker)
GOOS=linux GOARCH=amd64 go build -o vpn-commander .

# Build with optimizations
go build -ldflags="-w -s" -o vpn-commander .
```

### Testing

```bash
# Run tests
go test ./...

# Run tests with coverage
go test -race -coverprofile=coverage.txt ./...

# View coverage report
go tool cover -html=coverage.txt
```

### Docker Development

```bash
# Build development image
docker build -t vpn-commander:dev .

# Run with environment file
docker run --env-file .env vpn-commander:dev

# Run with debug logging
docker run -e LOG_LEVEL=debug --env-file .env vpn-commander:dev
```

## GitHub Packages Integration

This project automatically builds and publishes to GitHub Packages:

### Docker Images
- **Multi-architecture**: AMD64 and ARM64 support
- **Automatic tagging**: Based on Git tags and branches
- **Registry**: `ghcr.io/staners2/vpn-commander`

Available tags:
- `latest` - Latest stable release
- `main` - Latest commit on main branch  
- `v1.0.0` - Specific version tags
- `pr-123` - Pull request builds

### Helm Charts
- **OCI Registry**: Charts published as OCI artifacts
- **Registry**: `ghcr.io/staners2/vpn-commander/charts/vpn-commander`
- **Versioning**: Semantic versioning aligned with releases

### Usage Examples

**Pull specific version:**
```bash
docker pull ghcr.io/staners2/vpn-commander:v1.0.0
```

**Install specific chart version:**
```bash
helm install my-app oci://ghcr.io/staners2/vpn-commander/charts/vpn-commander --version 1.0.0
```

## CI/CD Pipeline

The project includes a comprehensive GitHub Actions pipeline:

### Pipeline Stages
1. **Testing**: Unit tests, code formatting, Go vet, staticcheck
2. **Security**: Gosec security scanning
3. **Build**: Multi-arch Docker builds with caching
4. **Helm**: Chart linting, templating, and publishing
5. **Release**: Automated releases with artifacts
6. **Vulnerability Scanning**: Container security scanning with Trivy

### Triggered On
- Push to `main` branch
- Pull requests
- Git tags (`v*`)

### Security Features
- **SARIF reports**: Vulnerability results uploaded to GitHub Security tab
- **Dependency scanning**: Automated dependency vulnerability checks
- **Multi-arch builds**: Support for different architectures

## Deployment Options Comparison

| Method | Pros | Cons | Best For |
|--------|------|------|----------|
| **Docker Compose** | Simple setup, easy management | Single host only | Home use, testing |
| **Kubernetes + Helm** | Scalable, production-ready | Complex setup | Production, enterprise |
| **Docker Run** | Quick start | Manual management | Development, demos |
| **Local Development** | Direct debugging | Requires Go setup | Development |

## Production Deployment

### Docker Compose Production Setup

```bash
# 1. Create production directory
mkdir -p /opt/vpn-commander
cd /opt/vpn-commander

# 2. Download files
curl -o docker-compose.yml https://raw.githubusercontent.com/staners2/vpn-commander/main/docker-compose.yml
curl -o .env https://raw.githubusercontent.com/staners2/vpn-commander/main/.env.example

# 3. Configure environment
nano .env  # Edit with your settings

# 4. Build and deploy
docker-compose up -d --build

# 5. Enable auto-start (systemd)
sudo tee /etc/systemd/system/vpn-commander.service << EOF
[Unit]
Description=VPN Commander Bot
Requires=docker.service
After=docker.service

[Service]
Type=forking
RemainAfterExit=yes
WorkingDirectory=/opt/vpn-commander
ExecStart=/usr/local/bin/docker-compose up -d --build
ExecStop=/usr/local/bin/docker-compose down
TimeoutStartSec=0
Restart=on-failure
StartLimitBurst=3

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable vpn-commander.service
sudo systemctl start vpn-commander.service
```

### Kubernetes Production Setup

**Prerequisites:**
- Kubernetes cluster (v1.20+)
- Helm 3.12+
- kubectl configured

**Deployment steps:**
```bash
# 1. Create namespace
kubectl create namespace vpn-commander

# 2. Pull chart
helm pull oci://ghcr.io/staners2/vpn-commander/charts/vpn-commander --untar

# 3. Create production values
cp vpn-commander/values.yaml values.prd.yaml

# Edit values.prd.yaml with your production configuration:
# - Set TELEGRAM_BOT_TOKEN from @BotFather
# - Set AUTH_CODE for secure bot authentication
# - Set ROUTER_HOST, ROUTER_USERNAME, ROUTER_PASSWORD
# - Update image.tag to specific version (e.g., "v1.0.0")
# - Adjust resource limits for production load
# - Configure nodeSelector, tolerations if needed

# 4. Deploy
helm install vpn-commander vpn-commander/ \
  -f values.prd.yaml \
  --namespace vpn-commander

# 5. Verify deployment
kubectl -n vpn-commander get pods
kubectl -n vpn-commander logs -f deployment/vpn-commander-vpn-commander
```

**Monitoring and maintenance:**
```bash
# Check pod status
kubectl -n vpn-commander get pods -l app.kubernetes.io/name=vpn-commander

# View logs
kubectl -n vpn-commander logs -f deployment/vpn-commander-vpn-commander

# Check health endpoint
kubectl -n vpn-commander port-forward deployment/vpn-commander-vpn-commander 8080:8080
curl http://localhost:8080/health

# Update deployment
helm upgrade vpn-commander vpn-commander/ \
  -f values.prd.yaml \
  --namespace vpn-commander

# Rollback if needed
helm rollback vpn-commander --namespace vpn-commander
```

## Monitoring and Logging

### Logs

The application uses structured logging with configurable levels:

```bash
# View logs in Kubernetes
kubectl logs -f deployment/vpn-commander

# View logs in Docker
docker logs -f vpn-commander
```

### Health Checks

The application includes health check endpoints:

- `/health`: Liveness probe endpoint
- `/ready`: Readiness probe endpoint

### Metrics

The application logs important metrics:
- SSH connection status
- VPN configuration changes
- User authentication attempts
- Error rates and types

## Troubleshooting

### Common Issues

1. **SSH Connection Failed**:
   - Check router IP address and credentials
   - Verify SSH is enabled on router
   - Check network connectivity

2. **Configuration Not Found**:
   - Verify Xray config path is correct
   - Check file permissions on router
   - Ensure Xray is properly installed

3. **Service Restart Failed**:
   - Check service name on your router
   - Verify user has sudo/root privileges
   - Check system logs on router

4. **Bot Not Responding**:
   - Verify Telegram bot token is correct
   - Check bot permissions and settings
   - Review application logs

### Debug Mode

Enable debug logging to get more detailed information:

```bash
# Set environment variable
export LOG_LEVEL=debug

# Or in Docker
docker run -e LOG_LEVEL=debug --env-file .env vpn-commander

# Or in Kubernetes
helm upgrade my-vpn-commander charts/vpn-commander --set env.LOG_LEVEL=debug
```

### Log Analysis

Important log fields to monitor:

- `level`: Log level (error, warn, info, debug)
- `msg`: Log message
- `user_id`: Telegram user ID (for user actions)
- `command`: SSH commands being executed
- `error`: Error details when things go wrong

## Security Best Practices

1. **Use Strong Authentication**:
   - Use a long, random auth code
   - Regularly rotate credentials
   - Monitor authentication attempts

2. **Secure SSH Access**:
   - Use key-based authentication when possible
   - Limit SSH access to specific IP ranges
   - Monitor SSH login attempts

3. **Network Security**:
   - Use firewalls to restrict access
   - Enable network policies in Kubernetes
   - Monitor network traffic

4. **Container Security**:
   - Run as non-root user
   - Use read-only root filesystem
   - Scan images for vulnerabilities

5. **Secret Management**:
   - Use Kubernetes secrets for sensitive data
   - Avoid logging sensitive information
   - Rotate secrets regularly

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

For support and questions:

1. Check the troubleshooting section
2. Review the logs for error details
3. Create an issue in the repository
4. Provide relevant logs and configuration (without sensitive data)

## Changelog

### v1.0.0
- Initial release
- Telegram bot with authentication
- VPN routing control
- SSH integration
- Docker and Kubernetes support
- CI/CD pipeline