package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"dagger.io/dagger"
	"github.com/digitalocean/godo"
	"github.com/felipepimentel/n8n-digitalocean-cicd/ci/ssh"
)

const (
	defaultDropletSize = "s-2vcpu-2gb"
	defaultRegion     = "nyc1"
	backupRetention   = 7 // days
)

type Config struct {
	doToken        string
	registryURL    string
	dropletName    string
	sshKeyID       string
	domain         string
	n8nVersion     string
	slackWebhook   string
	alertEmail     string
	encryptionKey  string
	basicAuthUser  string
	basicAuthPass  string
	sshKeyPath     string
}

func main() {
	ctx := context.Background()

	// Load configuration
	config := loadConfig()

	// Initialize DO client
	doClient := godo.NewFromToken(config.doToken)

	// Initialize Dagger client
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		panic(err)
	}
	defer client.Close()

	// Setup infrastructure
	dropletIP, err := setupInfrastructure(ctx, doClient, config)
	if err != nil {
		panic(err)
	}

	// Build and push N8N image
	if err := buildAndPushImage(ctx, client, config); err != nil {
		panic(err)
	}

	// Configure and deploy N8N
	if err := deployN8N(ctx, doClient, dropletIP, config); err != nil {
		panic(err)
	}

	fmt.Printf("N8N deployment completed successfully!\nAccess your instance at: https://%s\n", config.domain)
}

func loadConfig() Config {
	return Config{
		doToken:        requireEnv("DIGITALOCEAN_ACCESS_TOKEN"),
		registryURL:    requireEnv("DOCKER_REGISTRY"),
		dropletName:    requireEnvOrDefault("DROPLET_NAME", "n8n-server"),
		sshKeyID:       requireEnv("DO_SSH_KEY_ID"),
		domain:         requireEnv("N8N_DOMAIN"),
		n8nVersion:     requireEnvOrDefault("N8N_VERSION", "latest"),
		slackWebhook:   os.Getenv("SLACK_WEBHOOK_URL"),
		alertEmail:     os.Getenv("ALERT_EMAIL"),
		encryptionKey:  requireEnv("N8N_ENCRYPTION_KEY"),
		basicAuthUser:  requireEnv("N8N_BASIC_AUTH_USER"),
		basicAuthPass:  requireEnv("N8N_BASIC_AUTH_PASSWORD"),
		sshKeyPath:     requireEnv("DO_SSH_KEY_PATH"),
	}
}

func setupInfrastructure(ctx context.Context, client *godo.Client, config Config) (string, error) {
	// Create VPC if not exists
	vpc, err := createVPC(ctx, client, config)
	if err != nil {
		return "", err
	}

	// Create firewall
	if err := createFirewall(ctx, client, config, vpc.ID); err != nil {
		return "", err
	}

	// Create registry if not exists
	if err := createRegistry(ctx, client); err != nil {
		return "", err
	}

	// Create or get droplet
	droplet, err := createOrGetDroplet(ctx, client, config, vpc.ID)
	if err != nil {
		return "", err
	}

	// Configure DNS
	if err := configureDNS(ctx, client, config, droplet.Networks.V4[0].IPAddress); err != nil {
		return "", err
	}

	return droplet.Networks.V4[0].IPAddress, nil
}

func createVPC(ctx context.Context, client *godo.Client, config Config) (*godo.VPC, error) {
	vpcs, _, err := client.VPCs.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	vpcName := fmt.Sprintf("%s-vpc", config.dropletName)
	for _, vpc := range vpcs {
		if vpc.Name == vpcName {
			// Get the VPC by ID to ensure we have a proper pointer
			existingVPC, _, err := client.VPCs.Get(ctx, vpc.ID)
			return existingVPC, err
		}
	}

	createRequest := &godo.VPCCreateRequest{
		Name:        vpcName,
		RegionSlug:  defaultRegion,
		IPRange:     "10.10.10.0/24",
		Description: "VPC for n8n deployment",
	}

	vpc, _, err := client.VPCs.Create(ctx, createRequest)
	return vpc, err
}

func createFirewall(ctx context.Context, client *godo.Client, config Config, vpcID string) error {
	firewallName := fmt.Sprintf("%s-firewall", config.dropletName)
	
	request := &godo.FirewallRequest{
		Name: firewallName,
		InboundRules: []godo.InboundRule{
			{
				Protocol:  "tcp",
				PortRange: "22",
				Sources: &godo.Sources{
					Addresses: []string{"0.0.0.0/0"},
				},
			},
			{
				Protocol:  "tcp",
				PortRange: "80",
				Sources: &godo.Sources{
					Addresses: []string{"0.0.0.0/0"},
				},
			},
			{
				Protocol:  "tcp",
				PortRange: "443",
				Sources: &godo.Sources{
					Addresses: []string{"0.0.0.0/0"},
				},
			},
		},
		OutboundRules: []godo.OutboundRule{
			{
				Protocol: "tcp",
				PortRange: "1-65535",
				Destinations: &godo.Destinations{
					Addresses: []string{"0.0.0.0/0"},
				},
			},
		},
	}

	_, _, err := client.Firewalls.Create(ctx, request)
	return err
}

func createRegistry(ctx context.Context, client *godo.Client) error {
	_, _, err := client.Registry.Create(ctx, &godo.RegistryCreateRequest{
		Name:                 "n8n-registry",
		SubscriptionTierSlug: "basic",
	})
	return err
}

func createOrGetDroplet(ctx context.Context, client *godo.Client, config Config, vpcID string) (*godo.Droplet, error) {
	// Convert SSH key ID from string to int
	sshKeyID, err := strconv.Atoi(config.sshKeyID)
	if err != nil {
		return nil, fmt.Errorf("invalid SSH key ID: %v", err)
	}

	droplets, _, err := client.Droplets.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, droplet := range droplets {
		if droplet.Name == config.dropletName {
			return &droplet, nil
		}
	}

	createRequest := &godo.DropletCreateRequest{
		Name:   config.dropletName,
		Region: defaultRegion,
		Size:   defaultDropletSize,
		Image: godo.DropletCreateImage{
			Slug: "docker-20-04",
		},
		SSHKeys:           []godo.DropletCreateSSHKey{{ID: sshKeyID}},
		VPCUUID:          vpcID,
		Monitoring:       true,
		IPv6:            false,
		PrivateNetworking: true,
		UserData:         generateUserData(),
	}

	droplet, _, err := client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return nil, err
	}

	// Wait for droplet to be ready
	for {
		d, _, err := client.Droplets.Get(ctx, droplet.ID)
		if err != nil {
			return nil, err
		}
		if d.Status == "active" {
			return d, nil
		}
		time.Sleep(10 * time.Second)
	}
}

func configureDNS(ctx context.Context, client *godo.Client, config Config, ip string) error {
	// Extract domain and subdomain
	domain := config.domain
	
	createRecord := &godo.DomainRecordEditRequest{
		Type: "A",
		Name: "@",
		Data: ip,
		TTL:  3600,
	}

	_, _, err := client.Domains.CreateRecord(ctx, domain, createRecord)
	return err
}

func buildAndPushImage(ctx context.Context, client *dagger.Client, config Config) error {
	src := client.Host().Directory(".")

	// Create a timestamp for versioning
	timestamp := time.Now().Format("20060102150405")

	n8nImage := client.Container().
		From(fmt.Sprintf("n8nio/n8n:%s", config.n8nVersion)).
		WithEnvVariable("NODE_ENV", "production").
		WithEnvVariable("N8N_PORT", "5678").
		WithEnvVariable("N8N_PROTOCOL", "https").
		WithEnvVariable("N8N_METRICS", "true").
		WithEnvVariable("N8N_USER_FOLDER", "/home/node/.n8n").
		WithEnvVariable("N8N_ENCRYPTION_KEY", config.encryptionKey).
		WithEnvVariable("N8N_BASIC_AUTH_ACTIVE", "true").
		WithEnvVariable("N8N_BASIC_AUTH_USER", config.basicAuthUser).
		WithEnvVariable("N8N_BASIC_AUTH_PASSWORD", config.basicAuthPass).
		WithLabel("org.opencontainers.image.created", timestamp).
		WithLabel("org.opencontainers.image.version", config.n8nVersion).
		WithDirectory("/app", src)

	// Add security patches and updates
	n8nImage = n8nImage.
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "upgrade", "-y"}).
		WithExec([]string{"apt-get", "install", "-y", "curl", "ca-certificates", "jq", "fail2ban"}).
		WithExec([]string{"apt-get", "clean"}).
		WithExec([]string{"rm", "-rf", "/var/lib/apt/lists/*"})

	// Copy monitoring and backup scripts
	n8nImage = n8nImage.
		WithFile("/usr/local/bin/monitor.sh", client.Host().File("scripts/monitor.sh")).
		WithFile("/usr/local/bin/backup.sh", client.Host().File("scripts/backup.sh")).
		WithExec([]string{"chmod", "+x", "/usr/local/bin/monitor.sh", "/usr/local/bin/backup.sh"})

	// Push to registry with both latest and versioned tags
	baseRef := fmt.Sprintf("%s/n8n-app", config.registryURL)
	_, err := n8nImage.Publish(ctx, fmt.Sprintf("%s:latest", baseRef))
	if err != nil {
		return err
	}
	_, err = n8nImage.Publish(ctx, fmt.Sprintf("%s:%s", baseRef, timestamp))
	return err
}

func deployN8N(ctx context.Context, client *godo.Client, dropletIP string, config Config) error {
	// Generate deployment script
	deployScript := generateDeploymentScript(config)

	// Create SSH client
	sshClient, err := ssh.NewClient(dropletIP, 22, "root", config.sshKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %v", err)
	}

	// Execute deployment script via SSH
	output, err := sshClient.ExecuteCommand(deployScript)
	if err != nil {
		return fmt.Errorf("deployment failed: %v\nOutput: %s", err, output)
	}

	return nil
}

func generateUserData() string {
	return `#!/bin/bash
set -e

# System updates and Docker installation
apt-get update
apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    software-properties-common \
    fail2ban \
    ufw

# Install Docker
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io

# Configure UFW
ufw default deny incoming
ufw default allow outgoing
ufw allow ssh
ufw allow http
ufw allow https
yes | ufw enable

# Configure fail2ban
cat > /etc/fail2ban/jail.local << EOF
[sshd]
enabled = true
bantime = 3600
findtime = 600
maxretry = 3
EOF

systemctl enable fail2ban
systemctl start fail2ban

# Enable and start Docker
systemctl enable docker
systemctl start docker

# Create monitoring directories
mkdir -p /opt/n8n/{scripts,backups,logs}
`
}

func generateDeploymentScript(config Config) string {
	return fmt.Sprintf(`#!/bin/bash
set -e

# Login to registry
docker login registry.digitalocean.com -u %s -p %s

# Create Docker network
docker network create n8n-network || true

# Pull and run n8n
docker pull %s/n8n-app:latest

# Stop and remove existing container if it exists
docker rm -f n8n-container || true

# Run n8n with improved configuration
docker run -d \
    --name n8n-container \
    --restart unless-stopped \
    --network n8n-network \
    -p 80:5678 \
    -p 443:5678 \
    -v n8n_data:/home/node/.n8n \
    -v /opt/n8n/backups:/backups \
    --memory="2g" \
    --memory-reservation="1g" \
    --cpu-shares=1024 \
    --security-opt=no-new-privileges \
    --health-cmd="curl -f http://localhost:5678/healthz || exit 1" \
    --health-interval=1m \
    --health-timeout=10s \
    --health-retries=3 \
    -e N8N_ENCRYPTION_KEY="%s" \
    -e N8N_BASIC_AUTH_ACTIVE="true" \
    -e N8N_BASIC_AUTH_USER="%s" \
    -e N8N_BASIC_AUTH_PASSWORD="%s" \
    -e N8N_HOST="%s" \
    -e N8N_PROTOCOL="https" \
    -e N8N_PORT="5678" \
    -e NODE_ENV="production" \
    %s/n8n-app:latest

# Setup monitoring cron jobs
cat > /etc/cron.d/n8n-monitoring << EOF
*/5 * * * * root /usr/local/bin/monitor.sh >> /opt/n8n/logs/monitor.log 2>&1
0 3 * * * root /usr/local/bin/backup.sh >> /opt/n8n/logs/backup.log 2>&1
EOF

chmod 0644 /etc/cron.d/n8n-monitoring

# Wait for container to be healthy
echo "Waiting for n8n to be ready..."
timeout 300 bash -c 'until docker ps -f name=n8n-container --format "{{.Status}}" | grep -q "healthy"; do sleep 5; done'

echo "N8N deployment completed successfully!"
`, config.doToken, config.doToken, config.registryURL, config.encryptionKey, 
   config.basicAuthUser, config.basicAuthPass, config.domain, config.registryURL)
}

func requireEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("Environment variable %s is required", key))
	}
	return value
}

func requireEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
} 