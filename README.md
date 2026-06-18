# simple-discord-bot

## Development

### Setup

#### Installing Go

Follow instructions here https://go.dev/doc/install

Make sure you add Go to your PATH.

#### Installing Air

```bash
go install github.com/air-verse/air@latest
```

#### Installing Delve for Debugging

Follow install instructions here https://github.com/go-delve/delve/tree/master/Documentation/installation

```
go install github.com/go-delve/delve/cmd/dlv@latest
```

### Debugging

The `run-dev.sh` script will setup the paths when used to launch air.

Run `./run-dev.sh`

In VSCode press F5 or run 'Attach to Air' in the Run and Debug section.

You will need to restart the VSCode debugger on code changes since the hot-reloading that air does stops the remote debugger instance while it rebuilds.