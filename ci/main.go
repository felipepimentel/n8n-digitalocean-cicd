package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	registryRetryDelay      = 5 * time.Second

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

	// File permissions.
	sshDirPerm  = 0o700
	sshFilePerm = 0o600

	defaultGithubHome = "/home/runner"
	sshKeyName        = "id_rsa"
	sshDirName        = ".ssh"
)

var (
	ErrInvalidSSHKey       = errors.New("invalid SSH key ID")
	ErrSSHClient           = errors.New("failed to create SSH client")
	ErrDeployment          = errors.New("deployment failed")
	ErrEnvVarNotSet        = errors.New("environment variable not set")
	ErrEnvVarParseInt      = errors.New("failed to parse environment variable as integer")
	ErrDomainNotFound      = errors.New("domain not found")
	ErrDomainCreation      = errors.New("failed to create domain")
	ErrSSHKeyNotFound      = errors.New("SSH key not found")
	ErrDNSPropagation      = errors.New("timeout waiting for DNS propagation")
	ErrRegistryEmpty       = errors.New("registry creation failed: no registry name returned")
	ErrEmptyCredentials    = errors.New("empty registry credentials received")
	ErrRegistryNotReady    = errors.New("registry not ready after maximum retries")
	ErrInvalidSSHKeyFormat = errors.New("invalid SSH key format: key must begin with '-----BEGIN'")
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

	// Create SSH directory and key file with proper permissions
	sshPrivateKey := os.Getenv("DO_SSH_PRIVATE_KEY")
	if sshPrivateKey == "" {
		panic("DO_SSH_PRIVATE_KEY environment variable is required")
	}

	if err := setupSSHKey(config.sshKeyPath, sshPrivateKey); err != nil {
		panic(fmt.Sprintf("failed to setup SSH key: %v", err))
	}

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
	// Get home directory for SSH key path
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = defaultGithubHome // Default for GitHub Actions
	}

	defaultSSHPath := filepath.Join(homeDir, sshDirName, sshKeyName)

	return Config{
		doToken:        requireEnv("DIGITALOCEAN_ACCESS_TOKEN"),
		registryURL:    "registry.digitalocean.com",
		dropletName:    requireEnvOrDefault("DROPLET_NAME", "n8n-production"),
		sshFingerprint: requireEnv("DO_SSH_KEY_FINGERPRINT"),
		domain:         requireEnv("N8N_DOMAIN"),
		n8nVersion:     requireEnvOrDefault("N8N_VERSION", "latest"),
		slackWebhook:   os.Getenv("SLACK_WEBHOOK_URL"),
		alertEmail:     os.Getenv("ALERT_EMAIL"),
		encryptionKey:  requireEnv("N8N_ENCRYPTION_KEY"),
		basicAuthUser:  requireEnvOrDefault("N8N_BASIC_AUTH_USER", "admin"),
		basicAuthPass:  requireEnvOrDefault("N8N_BASIC_AUTH_PASS", "n8n-admin"),
		sshKeyPath:     requireEnvOrDefault("SSH_KEY_PATH", defaultSSHPath),
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
	registry, resp, err := client.Registry.Get(ctx)
	if err != nil {
		if resp == nil || resp.StatusCode != 404 {
			return fmt.Errorf("failed to check registry: %w", err)
		}

		// Registry doesn't exist, create it
		registry, _, err = client.Registry.Create(ctx, &godo.RegistryCreateRequest{
			Name:                 "n8n",
			SubscriptionTierSlug: "starter",
		})
		if err != nil {
			return fmt.Errorf("failed to create registry: %w", err)
		}
	}

	// Ensure we have a registry name
	if registry == nil || registry.Name == "" {
		return ErrRegistryEmpty
	}

	// Ensure registry is ready
	for i := 0; i < maxRetries; i++ {
		registry, _, err = client.Registry.Get(ctx)
		if err == nil && registry != nil && registry.Name != "" {
			return nil
		}

		time.Sleep(registryRetryDelay)
	}

	return ErrRegistryNotReady
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
	// First ensure registry exists
	doClient := godo.NewFromToken(config.doToken)
	err := createRegistry(ctx, doClient)

	if err != nil {
		return fmt.Errorf("failed to ensure registry exists: %w", err)
	}

	// Get registry credentials with read/write access
	credentials, _, err := doClient.Registry.DockerCredentials(ctx, &godo.RegistryDockerCredentialsRequest{
		ReadWrite: true,
	})

	if err != nil {
		return fmt.Errorf("failed to get registry credentials: %w", err)
	}

	if credentials == nil || len(credentials.DockerConfigJSON) == 0 {
		return ErrEmptyCredentials
	}

	// Create Docker config.json content with the registry credentials
	dockerConfigSecret := client.SetSecret("docker_config", string(credentials.DockerConfigJSON))

	// Get registry name
	registry, _, err := doClient.Registry.Get(ctx)

	if err != nil {
		return fmt.Errorf("failed to get registry info: %w", err)
	}

	if registry == nil || registry.Name == "" {
		return ErrRegistryEmpty
	}

	// Build base image URL
	baseRef := fmt.Sprintf("%s/%s", config.registryURL, registry.Name)

	// Create source directory
	src := client.Host().Directory(".")

	// Build the image
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
		WithEnvVariable("TINI_SUBREAPER", "true").
		WithEnvVariable("N8N_ENFORCE_SETTINGS_FILE_PERMISSIONS", "true").
		WithMountedSecret("/root/.docker/config.json", dockerConfigSecret).
		WithLabel("org.opencontainers.image.created", time.Now().Format(time.RFC3339)).
		WithLabel("org.opencontainers.image.version", config.n8nVersion).
		WithDirectory("/app", src)

	// Push latest tag
	latestRef := fmt.Sprintf("%s/n8n:latest", baseRef)
	_, err = n8nImage.Publish(ctx, latestRef)

	if err != nil {
		return fmt.Errorf("failed to publish latest image: %w", err)
	}

	// Push versioned tag
	versionedRef := fmt.Sprintf("%s/n8n:%s", baseRef, config.n8nVersion)
	_, err = n8nImage.Publish(ctx, versionedRef)

	if err != nil {
		return fmt.Errorf("failed to publish versioned image: %w", err)
	}

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
	return fmt.Sprintf("%s\n%s\n%s",
		generateDockerCompose(config),
		generateEnvFile(config),
		generateSetupCommands(config))
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
	// Optional email settings
	emailMode := os.Getenv("N8N_EMAIL_MODE")
	if emailMode == "" {
		emailMode = "false"
	}

	return fmt.Sprintf(`
# Create .env file for docker-compose
cat > /opt/n8n/.env << EOF
N8N_HOST=%s
N8N_ENCRYPTION_KEY=%s
DB_PASSWORD=$(openssl rand -hex 24)
N8N_BASIC_AUTH_USER=%s
N8N_BASIC_AUTH_PASSWORD=%s
N8N_EMAIL_MODE=%s
EOF`,
		config.domain,
		config.encryptionKey,
		config.basicAuthUser,
		config.basicAuthPass,
		emailMode)
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

func setupSSHKey(keyPath, privateKey string) error {
	absPath := keyPath

	// Always use absolute path
	if !filepath.IsAbs(keyPath) {
		homeDir := os.Getenv("HOME")
		if homeDir == "" {
			homeDir = defaultGithubHome
		}

		absPath = filepath.Join(homeDir, keyPath)
	}

	// Create .ssh directory if it doesn't exist
	sshDir := filepath.Dir(absPath)

	if err := os.MkdirAll(sshDir, sshDirPerm); err != nil {
		return fmt.Errorf("failed to create SSH directory: %w", err)
	}

	// Ensure the key is in the correct format (remove any extra newlines)
	cleanKey := strings.TrimSpace(privateKey)
	if !strings.HasPrefix(cleanKey, "-----BEGIN") {
		return ErrInvalidSSHKeyFormat
	}

	// Add newline at the end if missing
	if !strings.HasSuffix(cleanKey, "\n") {
		cleanKey += "\n"
	}

	// Create a temporary file for the original key
	tmpKeyPath := absPath + ".tmp"
	if err := os.WriteFile(tmpKeyPath, []byte(cleanKey), sshFilePerm); err != nil {
		return fmt.Errorf("failed to write temporary key file: %w", err)
	}
	defer os.Remove(tmpKeyPath)

	// Use openssl to convert the key to RSA format without passphrase
	cmd := exec.Command("openssl", "rsa", "-in", tmpKeyPath)
	output, err := cmd.Output()

	if err != nil {
		return fmt.Errorf("failed to convert key: %w\nOutput: %s", err, output)
	}

	// Write the converted key to the final location
	err = os.WriteFile(absPath, output, sshFilePerm)
	if err != nil {
		return fmt.Errorf("failed to write SSH key file: %w", err)
	}

	// Start ssh-agent and add the key
	startAgentCmd := `
eval "$(ssh-agent -s)"
ssh-add ` + absPath

	agentEnv := append(os.Environ(), "SSH_ASKPASS=/bin/false", "DISPLAY=")
	cmd = exec.Command("bash", "-c", startAgentCmd)
	cmd.Env = agentEnv

	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add key to ssh-agent: %w\nOutput: %s", err, output)
	}

	return nil
}
