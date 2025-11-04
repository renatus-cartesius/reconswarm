# ReconSwarm

ReconSwarm is a modular reconnaissance automation framework designed for distributed security testing. It provisions cloud infrastructure, executes parallel reconnaissance pipelines, and collects results with minimal configuration overhead.

ReconSwarm is suitable for bug bounty hunters, penetration testers, DevSecOps engineers, and security researchers who need scalable, automated reconnaissance workflows without manual infrastructure management.

## Features

- **Cloud-agnostic architecture** - Provisioner interface allows easy integration with multiple cloud providers (currently Yandex Cloud)
- **Flexible pipeline stages** - Extensible stage system supporting exec (command execution) and sync (file and directory synchronization) operations
- **Parallel execution** - Distributes reconnaissance tasks across multiple worker VMs with configurable concurrency
- **Automatic lifecycle management** - VM provisioning, setup, execution, and cleanup handled automatically
- **Template-based configuration** - Go templates for dynamic command and path generation
- **Multiple target types** - Support for crt.sh enumeration and manual target lists
- **Minimal dependencies** - Lightweight design with only essential dependencies

## Architecture

ReconSwarm follows a modular architecture with clear separation of concerns between cloud provisioning, remote system control, pipeline execution, and configuration management.

### Cloud Provider Abstraction

ReconSwarm uses a modular provisioning system that supports multiple cloud providers. Current implementations include Yandex Cloud. Additional cloud providers can be integrated by creating new provisioner implementations.

### Pipeline Stage System

Stages are extensible components that execute operations on worker VMs:

- **exec** - Execute shell commands with template support
- **sync** - Copy files or directories from remote VMs to local machine via SFTP (automatically detects file vs directory)

All stage fields support template rendering. New stage types can be added to extend functionality.

## Installation

```bash
git clone <repository>
cd reconswarm
go mod download
go build
```

## Configuration

ReconSwarm uses a YAML configuration file (`reconswarm.yaml` by default, configurable via `CONFIG_PATH` environment variable). All string values in the configuration file support environment variable expansion using `${VAR}` or `$VAR` syntax.

### Basic Configuration

```yaml
# Cloud provider credentials (can use environment variables)
iam_token: "${YC_TOKEN}"  # or use $YC_TOKEN
folder_id: "${YC_FOLDER_ID}"  # or use $YC_FOLDER_ID

# VM defaults (can also use environment variables)
default_zone: "${YC_ZONE}"  # or use "ru-central1-b" directly
default_image: "fd8b1cmhmncn7lt4tqn4"
default_username: "root"
default_cores: 2
default_memory: 2      # GB
default_disk_size: 20  # GB

# Worker pool configuration
max_workers: 5

# VM setup commands (executed once per VM, supports env vars)
setup_commands:
  - "apt update"
  - "apt install -y docker.io"

# Pipeline definition
pipeline:
  targets:
    - value: "example.com"
      type: crtsh
  stages:
    - name: "Run scanner"
      type: exec
      steps:
        - "nmap -sC -sV {{.Targets.filepath}}"
```

### Environment Variables

Configuration values support environment variable substitution in two formats:
- `${VAR}` - Full variable name in braces
- `$VAR` - Simple variable name

If an environment variable is not set, the literal string (including `${VAR}` or `$VAR`) will be used.

For `iam_token` and `folder_id`, the tool also checks `YC_TOKEN` and `YC_FOLDER_ID` environment variables directly. If these are set, they will override any values specified in the YAML file.

### Yandex Cloud Setup

For Yandex Cloud integration, use the provided setup script:

1. **Install Yandex Cloud CLI** (if not already installed):
   ```bash
   # Follow official Yandex Cloud documentation for CLI installation
   ```

2. **Configure Yandex Cloud CLI**:
   ```bash
   yc config profile create <profile-name>
   yc config set cloud-id <your-cloud-id>
   yc config set folder-id <your-folder-id>
   ```

3. **Export credentials**:
   ```bash
   source ./secrets-setup.sh
   ```
   
   This script exports:
   - `YC_TOKEN` - IAM token for authentication
   - `YC_FOLDER_ID` - Folder ID for resource management
   - `YC_CLOUD_ID` - Cloud ID (if needed)

4. **Use in configuration**:
   
   Option 1: Use environment variables directly (recommended):
   ```yaml
   # iam_token and folder_id will be automatically taken from YC_TOKEN and YC_FOLDER_ID
   # No need to specify them if environment variables are set
   ```
   
   Option 2: Reference in YAML:
   ```yaml
   iam_token: "${YC_TOKEN}"
   folder_id: "${YC_FOLDER_ID}"
   ```

The `secrets-setup.sh` script automatically generates a fresh IAM token each time it's executed, ensuring secure authentication without hardcoding credentials.

### Target Types

**crt.sh enumeration**:
```yaml
targets:
  - value: "example.com"
    type: crtsh
```

**Manual list**:
```yaml
targets:
  - value: ["sub1.example.com", "sub2.example.com"]
    type: list
```

### Stage Configuration

All stage configuration fields support Go template syntax for dynamic value generation. Template variables are rendered at execution time with context data provided automatically.

**Template Context**

The following data is available in all stage templates:

- `{{.Targets.filepath}}` - Absolute path to the targets file on the remote VM (contains all targets assigned to the worker)
- `{{.Targets.list}}` - Array of target strings for programmatic access
- `{{.Worker.Name}}` - Unique identifier of the worker VM instance

**Exec stage** - Executes shell commands with template support:
```yaml
stages:
  - name: "Run tool"
    type: exec
    steps:
      - "docker run --rm -v /opt/recon:/data scanner:latest {{.Targets.filepath}}"
      - "cat /opt/recon/results.json"
```

All commands in the `steps` array are template-rendered before execution.

**Sync stage** - Copies files or directories from remote to local using SFTP. Automatically detects whether the path is a file or directory:
```yaml
stages:
  - name: "Collect results"
    type: sync
    src: "/opt/recon/results.json"
    dest: "./results/{{.Worker.Name}}.json"
  
  # Sync entire directory recursively
  - name: "Collect all results"
    type: sync
    src: "/opt/recon"
    dest: "./results/{{.Worker.Name}}"
```

Both `src` (remote path) and `dest` (local path) support template rendering for dynamic file paths. The sync stage automatically detects whether the source path is a file or directory and handles it accordingly.

## Usage

### Manual Pipeline Execution

The `manual` command executes the full reconnaissance pipeline:

```bash
reconswarm manual
```

This command:
1. Prepares targets (enumerates subdomains via crt.sh if needed)
2. Creates worker VMs based on `max_workers` configuration
3. Distributes targets across workers
4. Executes setup commands on each VM
5. Runs pipeline stages sequentially
6. Collects results via sync stages
7. Automatically deallocates all infrastructure after completion

The automatic infrastructure deallocation ensures complete autonomy - all cloud resources are provisioned, used, and destroyed without manual intervention, enabling fully automated reconnaissance workflows.

### Example Configurations

For complete pipeline examples, see the [`examples/pipelines`](examples/pipelines) directory.

**Basic subdomain enumeration and scanning**:
```yaml
pipeline:
  targets:
    - value: "example.com"
      type: crtsh
  stages:
    - name: "Scan targets"
      type: exec
      steps:
        - "nmap -sC -sV -iL {{.Targets.filepath}} -oN /opt/recon/nmap-{{.Worker.Name}}.txt"
    - name: "Collect results"
      type: sync
      src: "/opt/recon/nmap-{{.Worker.Name}}.txt"
      dest: "./results/nmap-{{.Worker.Name}}.txt"
```

Run with:
```bash
reconswarm manual
```

**Multiple targets with Docker-based scanning**:
```yaml
pipeline:
  targets:
    - value: "example.com"
      type: crtsh
    - value: ["api.example.com", "www.example.com"]
      type: list
  stages:
    - name: "Run nuclei scan"
      type: exec
      steps:
        - "docker run --rm -v /opt/recon:/data projectdiscovery/nuclei:latest -l {{.Targets.filepath}} -json -o /opt/recon/nuclei-{{.Worker.Name}}.json"
    - name: "Copy nuclei results"
      type: sync
      src: "/opt/recon/nuclei-{{.Worker.Name}}.json"
      dest: "./results/nuclei-{{.Worker.Name}}.json"
```

Run with:
```bash
reconswarm manual
```

**Custom toolchain with multiple stages**:
```yaml
setup_commands:
  - "apt update"
  - "apt install -y git golang"
  - "git clone https://github.com/projectdiscovery/subfinder.git"
  - "cd subfinder && go build"

pipeline:
  targets:
    - value: "example.com"
      type: crtsh
  stages:
    - name: "Additional enumeration"
      type: exec
      steps:
        - "cd subfinder && ./subfinder -dL {{.Targets.filepath}} -o /opt/recon/subfinder-{{.Worker.Name}}.txt"
    - name: "Merge targets"
      type: exec
      steps:
        - "cat {{.Targets.filepath}} /opt/recon/subfinder-{{.Worker.Name}}.txt | sort -u > /opt/recon/all-targets-{{.Worker.Name}}.txt"
    - name: "Scan merged targets"
      type: exec
      steps:
        - "nmap -sC -sV -iL /opt/recon/all-targets-{{.Worker.Name}}.txt -oN /opt/recon/scan-{{.Worker.Name}}.txt"
    - name: "Collect all results"
      type: sync
      src: "/opt/recon"
      dest: "./results/{{.Worker.Name}}"
```

Note: The sync stage automatically detects that `/opt/recon` is a directory and recursively copies all files and subdirectories to the local destination.

Run with:
```bash
reconswarm manual
```

### Other Commands

**Subdomain enumeration**:
```bash
reconswarm crtsh-dump example.com
```

Fetches and filters resolvable subdomains from crt.sh for a given domain.

## TODO

### Multi-Cloud Provider Support

- [ ] Add AWS (EC2) provisioner
- [ ] Add Google Cloud Platform (Compute Engine) provisioner
- [ ] Add Azure (Virtual Machines) provisioner
- [ ] Add DigitalOcean provisioner

### Extended Pipeline Stage Types

- [ ] Add `notify` stage - Send notifications or alerts (webhooks, email, Slack)
- [ ] Add `conditional` stage - Execute stages based on previous stage results
- [ ] Add `parallel` stage - Execute multiple operations concurrently on the same worker
- [ ] Add `retry` stage - Automatically retry failed operations with configurable backoff
- [ ] Add `timeout` stage - Set execution timeouts per stage
- [ ] Add `validate` stage - Validate results or conditions before proceeding

### Daemon Mode with Scheduled Execution

- [ ] Implement scheduled execution with cron-like expressions
- [ ] Add continuous monitoring mode for long-running processes
- [ ] Add event-driven triggers (webhooks, external events)
- [ ] Implement result persistence and execution history tracking
- [ ] Add built-in health checks and automatic recovery

### Alternative Result Storage Types

- [ ] Add object storage support (S3, GCS, Azure Blob Storage)
- [ ] Add database storage support (PostgreSQL, MySQL, MongoDB)
- [ ] Add message queue support (RabbitMQ, Kafka, Redis streams)
- [ ] Add API endpoint integration (custom HTTP POST)
- [ ] Add email notification support with attachments
- [ ] Add cloud logging integration (CloudWatch, Stackdriver, etc.)

### Additional Target Sources

- [ ] Add DNSDumpster target source
- [ ] Add Censys target source
- [ ] Add Shodan target source
- [ ] Add ZoomEye target source
- [ ] Add SecurityTrails target source
- [ ] Add PassiveTotal target source
- [ ] Add VirusTotal target source
- [ ] Add additional reconnaissance data sources

## License

MIT License. See [LICENSE](LICENSE) file for details.
