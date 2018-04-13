
<!--- This file is automatically generated by make gen-cli-docs; changes should be made in the go CLI command code (under cmd/kops) -->

## kops replace

Replace cluster resources.

### Synopsis


Replace a resource desired configuration by filename or stdin.

```
kops replace -f FILENAME
```

### Examples

```
  # Replace a cluster desired configuration using a YAML file
  kops replace -f my-cluster.yaml
  
  # Note, if the resource does not exist the command will error, use --force to provision resource
  kops replace -f my-cluster.yaml --force
```

### Options

```
  -f, --filename stringSlice   A list of one or more files separated by a comma.
      --force                  Force any changes, which will also create any non-existing resource
```

### Options inherited from parent commands

```
      --alsologtostderr                  log to standard error as well as files
      --config string                    config file (default is $HOME/.kops.yaml)
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files (default false)
      --name string                      Name of cluster. Overrides KOPS_CLUSTER_NAME environment variable
      --state string                     Location of state storage. Overrides KOPS_STATE_STORE environment variable
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO
* [kops](kops.md)	 - kops is Kubernetes ops.
