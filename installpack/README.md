# installpack (embedded install pack)

This directory is the `go:embed` target for the versioned **Naslos install
pack** (`naslos-install-pack-<version>.tar.gz`) published by the
`AessemOps/Naslos-Linux` release workflow.

The pack contents are gitignored on purpose: packs are build artifacts, not
source.

```bash
make fetch-pack PACK_VERSION=0.1.0     # download + sha256-verify + extract here
make fetch-latest-pack                 # newest Naslos-Linux vX.Y.Z pack
```

Only this `README.md` and `embed.go` are committed, so `//go:embed *` always
matches at least one file on a fresh checkout. After a fetch the directory
contains:

```
metadata.json
charts/naslos/...
machine-config/naslos-installer.yaml.tmpl
cilium/cilium.yaml
manifests/local-path-v0.0.26.yaml
schematic/naslos.yaml
```

The pack schema and the stable interfaces are in
[`docs/installer-contract.md`](https://github.com/AessemOps/Naslos-Linux/blob/master/docs/installer-contract.md)
in Naslos-Linux. The Go loader lives in `internal/installpack`.
