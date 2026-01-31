# preboot-debug

Debug layer that logs all preboot environment variables.

## Usage

Symlink into your project's layers directory with your desired order:

```bash
ln -s /path/to/examples/layers/00-preboot-debug layers/05-preboot-debug
```

Then run `tank start` to see the environment variables printed during instance creation.
