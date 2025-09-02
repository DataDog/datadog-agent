# Experimental Static Quality Gates

A modular, extensible implementation of static quality gates that provides detailed artifact measurement and reporting capabilities. This system enables cross-platform measurement of packages, Docker images, and MSI installers with comprehensive file inventories and analysis.

## 🎯 Overview

The experimental static quality gates system is a complete rewrite of the original gates implementation, designed with:

- **Modular Architecture**: Clean separation of concerns with focused modules
- **Cross-Platform Support**: Measurements work on any platform without OS-specific tools
- **Enhanced Reporting**: Detailed file inventories with checksums and analysis
- **Extensible Design**: Easy to add new artifact types and measurement strategies
- **Comprehensive Testing**: Full test coverage for all components

## 🏗️ Architecture

The system follows a composition-based architecture organized into focused modules:

```
tasks/static_quality_gates/experimental/
├── common/           # Shared infrastructure and utilities
│   ├── models.py     # Data models and protocols  
│   ├── config.py     # Configuration management
│   ├── utils.py      # File processing utilities
│   └── report.py     # Report building and serialization
├── processors/      # Artifact-specific measurement logic
│   ├── base.py      # ArtifactProcessor protocol
│   ├── package.py   # DEB/RPM packages
│   ├── docker.py    # Docker images
│   └── msi.py       # MSI packages (cross-platform)
├── measurers/       # High-level measurement facades
│   ├── universal.py # Universal measurement orchestrator
│   ├── package.py   # Package measurement facade
│   ├── docker.py    # Docker measurement facade
│   └── msi.py       # MSI measurement facade
└── tasks/           # Invoke task definitions
    ├── package.py   # Package measurement tasks
    ├── docker.py    # Docker measurement tasks
    └── msi.py       # MSI measurement tasks
```

### Core Design Patterns

1. **Strategy Pattern**: `ArtifactProcessor` implementations handle different artifact types
2. **Composition**: `UniversalArtifactMeasurer` composes different processors
3. **Facade Pattern**: `InPlace*Measurer` classes provide convenient interfaces
4. **Immutable Data**: All models are frozen dataclasses for thread safety

## 🚀 Quick Start

### Basic Usage

```python
from tasks.static_quality_gates.experimental import InPlacePackageMeasurer

# Measure a DEB package
measurer = InPlacePackageMeasurer()
report = measurer.measure_package(
    ctx=ctx,
    package_path="/path/to/package.deb",
    gate_name="static_quality_gate_agent_deb_amd64",
    build_job_name="agent_deb-x64-a7"
)

print(f"Wire size: {report.on_wire_size:,} bytes")
print(f"Disk size: {report.on_disk_size:,} bytes")
print(f"Files: {len(report.file_inventory):,}")
```

### Universal Measurement

```python
from tasks.static_quality_gates.experimental import UniversalArtifactMeasurer

# Auto-select processor based on gate name
measurer = UniversalArtifactMeasurer.create_for_gate("static_quality_gate_agent_msi")
report = measurer.measure_artifact(
    ctx=ctx,
    artifact_ref="/path/to/datadog-agent.msi",
    gate_name="static_quality_gate_agent_msi",
    build_job_name="windows_build"
)
```

### Invoke Tasks

```bash
# Measure a package locally
dda inv quality-gates.measure-package-local \
  --package-path /path/to/package.deb \
  --gate-name static_quality_gate_agent_deb_amd64

# Measure a Docker image
dda inv quality-gates.measure-image-local \
  --image-ref nginx:latest \
  --gate-name static_quality_gate_docker_agent_amd64

# Measure an MSI package
dda inv quality-gates.measure-msi-local \
  --msi-path /path/to/datadog-agent.msi \
  --gate-name static_quality_gate_agent_msi
```

## 📦 Artifact Support

### Package Support (DEB, RPM)

- **Wire Size**: Compressed package file size
- **Disk Size**: Extracted package contents size
- **File Inventory**: Complete file listing with sizes and checksums
- **Cross-Platform**: Works on any OS via package extraction

### Docker Image Support

- **Wire Size**: Manifest-based compressed size calculation
- **Disk Size**: Layer-by-layer uncompressed analysis
- **File Inventory**: Complete filesystem analysis across all layers
- **Layer Analysis**: Individual layer information and metadata

### MSI Package Support ⭐ NEW

- **Wire Size**: MSI file size (on-wire measurement)
- **Disk Size**: ZIP extraction size (cross-platform approach)
- **File Inventory**: Complete Windows package contents
- **Cross-Platform**: No Windows-specific tools required

#### MSI Measurement Approach

The MSI processor uses an innovative cross-platform approach:

1. **Wire Size**: Measure the actual MSI file size
2. **Disk Size**: Extract the corresponding ZIP file (created during build)
3. **File Inventory**: Generate from ZIP extraction contents

This approach avoids dependency on Windows-specific tools like `msiexec` while maintaining measurement accuracy.

## 🔧 Configuration

### Quality Gate Configuration

Gates are configured in `test/static/static_quality_gates.yml`:

```yaml
static_quality_gate_agent_msi:
  max_on_wire_size: "160 MiB"
  max_on_disk_size: "1000 MiB"

static_quality_gate_agent_deb_amd64:
  max_on_wire_size: "150 MiB"  
  max_on_disk_size: "600 MiB"
```

### Measurement Options

```python
report = measurer.measure_package(
    ctx=ctx,
    package_path="/path/to/package.deb",
    gate_name="static_quality_gate_agent_deb_amd64",
    build_job_name="test_job",
    max_files=20000,           # Limit file inventory size
    generate_checksums=True,   # Generate SHA256 checksums
    debug=True                 # Enable detailed logging
)
```

## 📊 Report Format

### InPlaceArtifactReport

```python
@dataclass(frozen=True)
class InPlaceArtifactReport:
    # Core identification
    artifact_path: str
    gate_name: str
    
    # Size measurements  
    on_wire_size: int
    on_disk_size: int
    max_on_wire_size: int
    max_on_disk_size: int
    
    # File inventory
    file_inventory: list[FileInfo]
    
    # Metadata
    measurement_timestamp: str
    pipeline_id: str
    commit_sha: str
    arch: str
    os: str
    build_job_name: str
    
    # Optional Docker metadata
    docker_info: DockerImageInfo | None = None
```

### YAML Output

Reports can be saved as YAML for analysis:

```yaml
artifact_path: "/path/to/datadog-agent.msi"
gate_name: "static_quality_gate_agent_msi"
on_wire_size: 150000000
on_disk_size: 800000000
max_on_wire_size: 160000000
max_on_disk_size: 1000000000
file_inventory:
  - relative_path: "Program Files/Datadog/Agent/agent.exe"
    size_bytes: 50000000
    checksum: "sha256:abc123..."
  - relative_path: "Program Files/Datadog/Agent/datadog.yaml"
    size_bytes: 2048
    checksum: "sha256:def456..."
measurement_timestamp: "2024-01-15T14:30:22.123456Z"
pipeline_id: "12345"
commit_sha: "abc123def"
arch: "amd64"
os: "windows"
build_job_name: "windows_msi_build"
```

## 🧪 Testing

### Running Tests

```bash
# Run all experimental gates tests
python -m pytest tasks/unit_tests/experimental_gates_tests.py -v

# Run specific test classes
python -m pytest tasks/unit_tests/experimental_gates_tests.py::TestInPlaceMsiMeasurer -v
python -m pytest tasks/unit_tests/experimental_gates_tests.py::TestMsiInvokeTask -v
```

### Test Coverage

The test suite provides comprehensive coverage:

- **Data Models**: Validation, immutability, property calculations
- **Processors**: Artifact measurement, error handling, file discovery
- **Measurers**: Integration testing, configuration management
- **Tasks**: Invoke command testing, output validation
- **MSI Support**: Cross-platform approach, ZIP+MSI logic

## 🔍 Debugging

### Debug Mode

Enable detailed logging with the `debug=True` parameter:

```python
report = measurer.measure_package(
    # ... other params ...
    debug=True
)
```

Debug output includes:
- File discovery patterns and results
- Extraction progress and file counts
- Size calculations and intermediate results
- Error details and troubleshooting hints

### Common Issues

#### MSI Measurement Issues

1. **Missing ZIP file**: Ensure both MSI and ZIP files are present
   ```
   ValueError: Could not find ZIP file matching pattern
   ```
   
2. **Permission errors**: Check file permissions in extraction directory
   ```
   RuntimeError: ZIP extraction failed with exit code 1
   ```

#### Package Measurement Issues  

1. **Missing packages**: Verify package file exists and is readable
   ```
   ValueError: Package file not found: /path/to/package.deb
   ```

2. **Extraction failures**: Check available disk space and permissions
   ```
   RuntimeError: Failed to extract package
   ```

#### Docker Measurement Issues

1. **Image not available**: Ensure image is pulled locally
   ```
   RuntimeError: Failed to ensure image is available
   ```

2. **Manifest inspection failures**: Check Docker CLI configuration
   ```
   RuntimeError: Docker manifest inspect failed
   ```

## 🔄 Migration from Legacy System

### Key Differences

| Aspect | Legacy System | Experimental System |
|--------|---------------|-------------------|
| Architecture | Inheritance-based | Composition-based |
| Modularity | Monolithic file | Focused modules |
| Testing | Limited coverage | Comprehensive tests |
| Reporting | Basic metrics | Detailed inventories |
| MSI Support | Windows-dependent | Cross-platform |
| Extensibility | Difficult | Plugin-friendly |

### Migration Path

1. **Import Update**: Use the new module structure
   ```python
   from tasks.static_quality_gates.experimental import InPlacePackageMeasurer
   ```

2. **API Compatibility**: The public APIs remain largely compatible
3. **Configuration**: No changes required to YAML configuration
4. **New Features**: MSI support and enhanced reporting available immediately

## 🤝 Contributing

### Adding New Artifact Types

1. **Create Processor**: Implement `ArtifactProcessor` protocol
   ```python
   class NewTypeProcessor:
       def measure_artifact(self, ctx, artifact_ref, gate_config, 
                          max_files, generate_checksums, debug):
           # Implementation here
           return wire_size, disk_size, file_inventory, metadata
   ```

2. **Create Measurer**: Add facade class for convenience
3. **Add Tests**: Comprehensive test coverage required  
4. **Update Factory**: Add logic to `UniversalArtifactMeasurer.create_for_gate()`

### Code Style

- Follow existing patterns and naming conventions
- Use type hints for all function signatures
- Prefer composition over inheritance
- Write comprehensive docstrings
- Add debug logging for troubleshooting

## 📄 License

This code is part of the Datadog Agent project and subject to the same license terms.

## 🔗 Related Documentation

- [Static Quality Gates Confluence](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates)
- [Toolbox Page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox)
- [Original Gates Implementation](../gates.py)
