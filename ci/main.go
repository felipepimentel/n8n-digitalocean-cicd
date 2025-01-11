package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"dagger.io/dagger"
	"github.com/digitalocean/godo"

	"github.com/felipepimentel/n8n-digitalocean-cicd/ci/ssh"
)

const (
	defaultDropletSize      = "s-2vcpu-2gb"
	defaultRegion           = "nyc1"
	backupRetention         = 7 // days.
	sshPort                 = 22
	dnsRecordTTL            = 3600
	healthCheckDelay        = 10 * time.Second
	dropletStatusCheckDelay = 5 * time.Second
	maxRetries              = 3

	// DNS configuration.
	dnsCheckInterval    = 10 * time.Second
	dnsTimeout          = 5 * time.Minute
	dnsHealthCheckDelay = 30 * time.Second

	// Resource limits.
	cpuLimit          = "2"
	memoryLimit       = "2G"
	cpuReservation    = "1"
	memoryReservation = "1G"

	// Magic numbers.
	minDomainParts = 2
	sshReadyDelay  = 30 * time.Second
)

var (
	ErrInvalidSSHKey  = errors.New("invalid SSH key ID")
	ErrSSHClient      = errors.New("failed to create SSH client")
	ErrDeployment     = errors.New("deployment failed")
	ErrEnvVarNotSet   = errors.New("environment variable not set")
	ErrEnvVarParseInt = errors.New("failed to parse environment variable as integer")
	ErrDomainNotFound = errors.New("domain not found")
	ErrDomainCreation = errors.New("failed to create domain")
	ErrSSHKeyNotFound = errors.New("SSH key not found")
	ErrDNSPropagation = errors.New("timeout waiting for DNS propagation")
)

type Config struct {
	doToken        string
	registryURL    string
	dropletName    string
	sshFingerprint string
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
	dropletIP, err := setupInfrastructure(ctx, doClient, &config)
	if err != nil {
		panic(err)
	}

	// Build and push N8N image
	if err := buildAndPushImage(ctx, client, &config); err != nil {
		panic(err)
	}

	// Configure and deploy N8N
	if err := deployN8N(dropletIP, &config); err != nil {
		panic(err)
	}

	fmt.Printf("N8N deployment completed successfully!\nAccess your instance at: https://%s\n", config.domain)
}

func loadConfig() Config {
	// Required environment variables
	requiredVars := []string{
		"DIGITALOCEAN_ACCESS_TOKEN",
		"DO_SSH_KEY_FINGERPRINT",
		"N8N_DOMAIN",
		"N8N_ENCRYPTION_KEY",
	}

	var missingVars []string

	for _, varName := range requiredVars {
		if os.Getenv(varName) == "" {
			missingVars = append(missingVars, varName)
		}
	}

	if len(missingVars) > 0 {
		fmt.Println("\nError: Missing required environment variables:")

		for _, varName := range missingVars {
			fmt.Printf("- %s\n", varName)
		}

		fmt.Println("\nPlease set all required environment variables before running the deployment")
		panic("\nMissing required environment variables")
	}

	// Optional parameters with defaults
	registryURL := requireEnvOrDefault("DOCKER_REGISTRY", "registry.digitalocean.com")
	dropletName := requireEnvOrDefault("DROPLET_NAME", "n8n-server")
	n8nVersion := requireEnvOrDefault("N8N_VERSION", "latest")
	basicAuthUser := requireEnvOrDefault("N8N_BASIC_AUTH_USER", "admin")
	basicAuthPass := requireEnvOrDefault("N8N_BASIC_AUTH_PASSWORD", os.Getenv("N8N_ENCRYPTION_KEY"))
	sshKeyPath := requireEnvOrDefault("DO_SSH_KEY_PATH", "~/.ssh/id_rsa")

	// Optional monitoring parameters
	slackWebhook := os.Getenv("SLACK_WEBHOOK_URL")
	alertEmail := os.Getenv("ALERT_EMAIL")

	return Config{
		doToken:        os.Getenv("DIGITALOCEAN_ACCESS_TOKEN"),
		registryURL:    registryURL,
		dropletName:    dropletName,
		sshFingerprint: os.Getenv("DO_SSH_KEY_FINGERPRINT"),
		domain:         os.Getenv("N8N_DOMAIN"),
		n8nVersion:     n8nVersion,
		slackWebhook:   slackWebhook,
		alertEmail:     alertEmail,
		encryptionKey:  os.Getenv("N8N_ENCRYPTION_KEY"),
		basicAuthUser:  basicAuthUser,
		basicAuthPass:  basicAuthPass,
		sshKeyPath:     sshKeyPath,
	}
}

func setupInfrastructure(ctx context.Context, client *godo.Client, config *Config) (string, error) {
	// Ensure SSH key exists
	sshKeyID, err := ensureSSHKey(ctx, client, config)
	if err != nil {
		return "", fmt.Errorf("failed to ensure SSH key: %w", err)
	}

	// Create VPC if not exists
	vpc, err := createVPC(ctx, client, config)
	if err != nil {
		return "", err
	}

	// Create firewall
	err = createFirewall(ctx, client, config)
	if err != nil {
		return "", err
	}

	// Create registry if not exists
	err = createRegistry(ctx, client)
	if err != nil {
		return "", err
	}

	// Ensure domain exists
	err = ensureDomain(ctx, client, config)
	if err != nil {
		return "", fmt.Errorf("failed to ensure domain: %w", err)
	}

	// Create or get droplet
	droplet, err := createOrGetDroplet(ctx, client, config, vpc.ID, sshKeyID)
	if err != nil {
		return "", err
	}

	// Configure DNS with health check
	err = configureAndVerifyDNS(ctx, client, config, droplet)
	if err != nil {
		return "", err
	}

	return droplet.Networks.V4[0].IPAddress, nil
}

func ensureSSHKey(ctx context.Context, client *godo.Client, config *Config) (int, error) {
	// First try to find existing key by fingerprint
	keys, _, err := client.Keys.List(ctx, &godo.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to list SSH keys: %w", err)
	}

	for _, key := range keys {
		if key.Fingerprint == config.sshFingerprint {
			return key.ID, nil
		}
	}

	// If key not found, try to read from file and create it
	keyBytes, err := os.ReadFile(os.ExpandEnv(config.sshKeyPath))
	if err != nil {
		return 0, fmt.Errorf("failed to read SSH key file: %w", err)
	}

	createRequest := &godo.KeyCreateRequest{
		Name:      fmt.Sprintf("%s-key", config.dropletName),
		PublicKey: string(keyBytes),
	}

	key, _, err := client.Keys.Create(ctx, createRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to create SSH key: %w", err)
	}

	return key.ID, nil
}

func getDomainParts(domain string) (rootDomain string, parts []string) {
	parts = strings.Split(domain, ".")
	rootDomain = domain

	if len(parts) > minDomainParts {
		rootDomain = strings.Join(parts[len(parts)-minDomainParts:], ".")
	}

	return rootDomain, parts
}

func ensureDomain(ctx context.Context, client *godo.Client, config *Config) error {
	rootDomain, _ := getDomainParts(config.domain)

	// Check if domain exists
	_, resp, err := client.Domains.Get(ctx, rootDomain)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			// Domain doesn't exist, create it
			_, _, createErr := client.Domains.Create(ctx, &godo.DomainCreateRequest{
				Name: rootDomain,
			})

			if createErr != nil {
				return fmt.Errorf("%w: %s", ErrDomainCreation, createErr)
			}

			return nil
		}

		return fmt.Errorf("failed to check domain: %w", err)
	}

	return nil
}

func sanitizeRecordName(name string) string {
	// If name is empty or root domain, return @
	if name == "" || name == "@" {
		return "@"
	}

	// Replace invalid characters with -
	invalidChars := regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	sanitized := invalidChars.ReplaceAllString(name, "-")

	return sanitized
}

func configureAndVerifyDNS(ctx context.Context, client *godo.Client, config *Config, droplet *godo.Droplet) error {
	recordName := "@"
	rootDomain := config.domain
	parts := strings.Split(config.domain, ".")

	if len(parts) > minDomainParts {
		recordName = sanitizeRecordName(parts[0])
		rootDomain = strings.Join(parts[len(parts)-minDomainParts:], ".")
	}

	// Create or update A record
	createRequest := &godo.DomainRecordEditRequest{
		Type: "A",
		Name: recordName,
		Data: droplet.Networks.V4[0].IPAddress,
		TTL:  dnsRecordTTL,
	}

	_, _, err := client.Domains.CreateRecord(ctx, rootDomain, createRequest)
	if err != nil {
		return fmt.Errorf("failed to create DNS record: %w", err)
	}

	// Wait for DNS propagation
	return waitForDNSPropagation(ctx)
}

func waitForDNSPropagation(ctx context.Context) error {
	ticker := time.NewTicker(dnsCheckInterval)
	defer ticker.Stop()

	timeout := time.After(dnsTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return ErrDNSPropagation
		case <-ticker.C:
			// Implement DNS lookup here to verify propagation
			// For now we'll just wait a reasonable time
			time.Sleep(dnsHealthCheckDelay)
			return nil
		}
	}
}

func createVPC(ctx context.Context, client *godo.Client, config *Config) (*godo.VPC, error) {
	vpcs, _, err := client.VPCs.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	vpcName := fmt.Sprintf("%s-vpc", config.dropletName)

	for i := range vpcs {
		if vpcs[i].Name == vpcName {
			existingVPC, _, getErr := client.VPCs.Get(ctx, vpcs[i].ID)
			if getErr != nil {
				return nil, getErr
			}

			return existingVPC, nil
		}
	}

	createRequest := &godo.VPCCreateRequest{
		Name:        vpcName,
		RegionSlug:  defaultRegion,
		IPRange:     "192.168.32.0/24",
		Description: "VPC for n8n deployment",
	}

	vpc, _, err := client.VPCs.Create(ctx, createRequest)
	if err != nil {
		return nil, err
	}

	return vpc, nil
}

func createFirewall(ctx context.Context, client *godo.Client, config *Config) error {
	firewallName := fmt.Sprintf("%s-firewall", config.dropletName)

	// Check if firewall already exists
	firewalls, _, err := client.Firewalls.List(ctx, &godo.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list firewalls: %w", err)
	}

	for i := range firewalls {
		if firewalls[i].Name == firewallName {
			// Firewall exists, update it
			updateRequest := &godo.FirewallRequest{
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
						Protocol:  "tcp",
						PortRange: "1-65535",
						Destinations: &godo.Destinations{
							Addresses: []string{"0.0.0.0/0"},
						},
					},
				},
			}

			_, _, err = client.Firewalls.Update(ctx, firewalls[i].ID, updateRequest)
			if err != nil {
				return fmt.Errorf("failed to update firewall: %w", err)
			}

			return nil
		}
	}

	// Create new firewall if it doesn't exist
	createRequest := &godo.FirewallRequest{
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
				Protocol:  "tcp",
				PortRange: "1-65535",
				Destinations: &godo.Destinations{
					Addresses: []string{"0.0.0.0/0"},
				},
			},
		},
	}

	_, _, err = client.Firewalls.Create(ctx, createRequest)
	if err != nil {
		return fmt.Errorf("failed to create firewall: %w", err)
	}

	return nil
}

func createRegistry(ctx context.Context, client *godo.Client) error {
	// Check if registry already exists
	_, resp, err := client.Registry.Get(ctx)
	if err != nil {
		if resp == nil || resp.StatusCode != 404 {
			return fmt.Errorf("failed to check registry: %w", err)
		}

		// Registry doesn't exist, create it
		_, _, err = client.Registry.Create(ctx, &godo.RegistryCreateRequest{
			Name:                 "n8n-registry",
			SubscriptionTierSlug: "basic",
		})
		if err != nil {
			return fmt.Errorf("failed to create registry: %w", err)
		}
	}

	return nil
}

func createOrGetDroplet(ctx context.Context, client *godo.Client, config *Config, vpcID string, sshKeyID int) (*godo.Droplet, error) {
	// Check if droplet already exists
	droplets, _, err := client.Droplets.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list droplets: %w", err)
	}

	// Use index to avoid copying large structs
	for i := range droplets {
		if droplets[i].Name == config.dropletName {
			return &droplets[i], nil
		}
	}

	// Create new droplet using Docker marketplace image
	createRequest := &godo.DropletCreateRequest{
		Name:   config.dropletName,
		Region: defaultRegion,
		Size:   defaultDropletSize,
		Image: godo.DropletCreateImage{
			Slug: "docker-20-04", // Docker marketplace image
		},
		SSHKeys: []godo.DropletCreateSSHKey{
			{
				ID: sshKeyID,
			},
		},
		Monitoring: true,
		VPCUUID:    vpcID,
		Tags:       []string{"n8n", "production"},
		IPv6:       true,
		Backups:    true,
		UserData:   generateUserData(config), // Script to run on first boot
	}

	droplet, _, err := client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to create droplet: %w", err)
	}

	// Wait for droplet to be ready
	for {
		d, _, err := client.Droplets.Get(ctx, droplet.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get droplet status: %w", err)
		}

		if d.Status == "active" {
			// Wait a bit more to ensure SSH is ready
			time.Sleep(sshReadyDelay)

			// Configure non-root user
			if err := setupNonRootUser(d.Networks.V4[0].IPAddress, config); err != nil {
				return nil, fmt.Errorf("failed to setup non-root user: %w", err)
			}

			return d, nil
		}

		time.Sleep(dropletStatusCheckDelay)
	}
}

func setupNonRootUser(dropletIP string, config *Config) error {
	// Create SSH client as root
	sshClient, err := ssh.NewClient(dropletIP, sshPort, "root", config.sshKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}

	// Create n8n user and setup
	setupScript := `
#!/bin/bash
set -e

# Create n8n user
useradd -m -s /bin/bash n8n

# Add to sudo group
usermod -aG sudo n8n
usermod -aG docker n8n

# Set up SSH directory
mkdir -p /home/n8n/.ssh
chmod 700 /home/n8n/.ssh

# Copy SSH key
cp /root/.ssh/authorized_keys /home/n8n/.ssh/
chown -R n8n:n8n /home/n8n/.ssh
chmod 600 /home/n8n/.ssh/authorized_keys

# Set up sudoers
echo "n8n ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/n8n
chmod 440 /etc/sudoers.d/n8n

# Create necessary directories
mkdir -p /opt/n8n/{caddy_config,local_files}
chown -R n8n:n8n /opt/n8n

# Create docker volumes
docker volume create caddy_data
docker volume create n8n_data

# Set proper permissions
chown -R n8n:n8n /opt/n8n
`

	if _, err := sshClient.ExecuteCommand(setupScript); err != nil {
		return fmt.Errorf("failed to execute setup script: %w", err)
	}

	return nil
}

func generateUserData(_ *Config) string {
	return `#!/bin/bash
set -e

# System updates
apt-get update
apt-get upgrade -y

# Install required packages
apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    software-properties-common \
    fail2ban \
    ufw \
    git \
    jq

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

# Create app directories
mkdir -p /opt/n8n/{caddy_config,local_files}

# Clone n8n-docker-caddy repository
cd /opt/n8n
git clone https://github.com/n8n-io/n8n-docker-caddy.git
mv n8n-docker-caddy/* .
rm -rf n8n-docker-caddy

# Create Caddyfile
cat > /opt/n8n/caddy_config/Caddyfile << EOF
${config.domain} {
    reverse_proxy n8n:5678 {
        flush_interval -1
    }
}
EOF
`
}

func buildAndPushImage(ctx context.Context, client *dagger.Client, config *Config) error {
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

	// Add security patches and updates using Alpine package manager
	n8nImage = n8nImage.
		WithExec([]string{"/bin/sh", "-c", "apk update"}).
		WithExec([]string{"/bin/sh", "-c", "apk upgrade"}).
		WithExec([]string{"/bin/sh", "-c", "apk add curl ca-certificates jq"}).
		WithExec([]string{"/bin/sh", "-c", "rm -rf /var/cache/apk/*"})

	// Push to registry with both latest and versioned tags
	baseRef := fmt.Sprintf("%s/n8n-app", config.registryURL)

	// Push latest tag
	_, err := n8nImage.Publish(ctx, fmt.Sprintf("%s:latest", baseRef))
	if err != nil {
		time.Sleep(time.Second)
		return fmt.Errorf("failed to publish latest image: %w", err)
	}

	// Push versioned tag
	_, err = n8nImage.Publish(ctx, fmt.Sprintf("%s:%s", baseRef, timestamp))
	if err != nil {
		time.Sleep(time.Second)
		return fmt.Errorf("failed to publish versioned image: %w", err)
	}

	time.Sleep(time.Second)

	return nil
}

func deployN8N(dropletIP string, config *Config) error {
	// Generate deployment script
	deployScript := generateDeploymentScript(config)

	// Create SSH client
	sshClient, err := ssh.NewClient(dropletIP, sshPort, "root", config.sshKeyPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSSHClient, err)
	}

	// Execute deployment script via SSH
	output, err := sshClient.ExecuteCommand(deployScript)
	if err != nil {
		return fmt.Errorf("%w: %v\nOutput: %s", ErrDeployment, err, output)
	}

	return nil
}

func generateDeploymentScript(config *Config) string {
	return fmt.Sprintf("%s\n%s\n%s\n%s",
		generateDockerCompose(config),
		generateEnvFile(config),
		generateSetupCommands(config),
		generateBackupConfig(),
	)
}

func generateDockerCompose(config *Config) string {
	return fmt.Sprintf(`#!/bin/bash
set -e

# Create docker-compose.yml
cat > /opt/n8n/docker-compose.yml << 'EOF'
%s
EOF`, generateDockerComposeContent(config))
}

func generateDockerComposeContent(config *Config) string {
	return generatePostgresCheck() + "\n" + generateServicesConfig(config)
}

func generatePostgresCheck() string {
	return `
# Check if PostgreSQL container exists and is running
if docker ps -a --format '{{.Names}}' | grep -q "^n8n-db-1$"; then
	echo "PostgreSQL container already exists, skipping creation..."
	POSTGRES_EXISTS=true
else
	POSTGRES_EXISTS=false
fi`
}

func generateServicesConfig(config *Config) string {
	return fmt.Sprintf(`version: '3.8'

services:
  n8n:%s
  db:%s
  caddy:%s

volumes:
  n8n_data:
  db_data:
  caddy_data:
  caddy_config:

networks:
  n8n_network:
    driver: bridge`,
		generateN8NServiceConfig(config),
		generateDBServiceConfig(),
		generateCaddyServiceConfig())
}

func generateN8NServiceConfig(config *Config) string {
	return fmt.Sprintf(`
    image: %s/n8n-app:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:5678:5678"
    environment:
      - N8N_HOST=${N8N_HOST}
      - N8N_PORT=5678
      - N8N_PROTOCOL=https
      - NODE_ENV=production
      - N8N_ENCRYPTION_KEY=${N8N_ENCRYPTION_KEY}
      - DB_TYPE=postgresdb
      - DB_POSTGRESDB_HOST=db
      - DB_POSTGRESDB_DATABASE=n8n
      - DB_POSTGRESDB_USER=n8n
      - DB_POSTGRESDB_PASSWORD=${DB_PASSWORD}
      - N8N_EMAIL_MODE=${N8N_EMAIL_MODE}
      - N8N_SMTP_HOST=${N8N_SMTP_HOST}
      - N8N_SMTP_PORT=${N8N_SMTP_PORT}
      - N8N_SMTP_USER=${N8N_SMTP_USER}
      - N8N_SMTP_PASS=${N8N_SMTP_PASS}
      - N8N_SMTP_SENDER=${N8N_SMTP_SENDER}
      - WEBHOOK_URL=${WEBHOOK_URL}
      - N8N_BASIC_AUTH_ACTIVE=true
      - N8N_BASIC_AUTH_USER=${N8N_BASIC_AUTH_USER}
      - N8N_BASIC_AUTH_PASSWORD=${N8N_BASIC_AUTH_PASSWORD}
      - N8N_HIRING_BANNER_ENABLED=false
      - N8N_DIAGNOSTICS_ENABLED=false
      - N8N_METRICS=true
    volumes:
      - n8n_data:/home/node/.n8n
      - /opt/n8n/local_files:/files
    depends_on:
      - db
    networks:
      - n8n_network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:5678/healthz"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 30s
    deploy:
      resources:
        limits:
          cpus: '%s'
          memory: %s
        reservations:
          cpus: '%s'
          memory: %s'`, config.registryURL, cpuLimit, memoryLimit, cpuReservation, memoryReservation)
}

func generateDBServiceConfig() string {
	return `
    image: postgres:13
    restart: unless-stopped
    environment:
      - POSTGRES_DB=n8n
      - POSTGRES_USER=n8n
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - db_data:/var/lib/postgresql/data
    networks:
      - n8n_network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U n8n"]
      interval: 10s
      timeout: 5s
      retries: 5
    profiles:
      - new-install`
}

func generateCaddyServiceConfig() string {
	return `
    image: caddy:2
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /opt/n8n/caddy_config/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    networks:
      - n8n_network
    depends_on:
      - n8n`
}

func generateEnvFile(config *Config) string {
	return fmt.Sprintf(`
# Create .env file for docker-compose
cat > /opt/n8n/.env << EOF
N8N_HOST=%s
N8N_ENCRYPTION_KEY=%s
DB_PASSWORD=$(openssl rand -hex 24)
N8N_BASIC_AUTH_USER=%s
N8N_BASIC_AUTH_PASSWORD=%s
N8N_EMAIL_MODE=%s
N8N_SMTP_HOST=%s
N8N_SMTP_PORT=%s
N8N_SMTP_USER=%s
N8N_SMTP_PASS=%s
N8N_SMTP_SENDER=%s
WEBHOOK_URL=%s
EOF`,
		config.domain,
		config.encryptionKey,
		config.basicAuthUser,
		config.basicAuthPass,
		os.Getenv("N8N_EMAIL_MODE"),
		os.Getenv("N8N_SMTP_HOST"),
		os.Getenv("N8N_SMTP_PORT"),
		os.Getenv("N8N_SMTP_USER"),
		os.Getenv("N8N_SMTP_PASS"),
		os.Getenv("N8N_SMTP_SENDER"),
		config.slackWebhook)
}

func generateSetupCommands(config *Config) string {
	return fmt.Sprintf(`
# Set proper permissions
chown -R n8n:n8n /opt/n8n
chmod 600 /opt/n8n/.env

# Login to registry
docker login registry.digitalocean.com -u %s -p %s

# Pull and start services
cd /opt/n8n

# Start services based on PostgreSQL existence
if [ "$POSTGRES_EXISTS" = true ]; then
	docker-compose pull n8n caddy
	docker-compose up -d n8n caddy
else
	docker-compose pull
	docker-compose --profile new-install up -d
fi

# Wait for services to be healthy
echo "Waiting for services to be ready..."
timeout 300 bash -c 'until docker-compose ps | grep -q "(healthy)"; do sleep 5; done'`,
		config.doToken,
		config.doToken)
}

func generateBackupConfig() string {
	return `
# Setup backup cron job
cat > /etc/cron.d/n8n-backup << EOF
0 3 * * * n8n cd /opt/n8n && docker-compose exec -T db pg_dump -U n8n n8n > /opt/n8n/backups/n8n-\$(date +\%%Y\%%m\%%d).sql
EOF
chmod 0644 /etc/cron.d/n8n-backup

echo "N8N deployment completed successfully!"
`
}

func requireEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("Environment variable %s is required", key))
	}

	time.Sleep(time.Second)

	return value
}

func requireEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	time.Sleep(time.Second)

	return value
}
