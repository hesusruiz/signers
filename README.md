# signers
`signers` is a program to monitor the block generation and signing activity of the Validator nodes in the Alastria RedT blockchain network.

The program is a standalone executable with no dependencies (it is implemented in Go), is multiplatform and runs on any standard terminal window.
It uses the standard JSON-RPC APIs of Quorum and it can connect to the node with several mechanisms:

- WebSockets in event-driven mode if the blockchain node has it enabled and accessible for the `signers` program. Access can be remote or local in the same server where the blockchain node is running (e.g., running `signers` via SSH).
- HTTP in polling mode. Access to the node can also be remote or local. The polling interval can be configured via command line parameter. Independent from the polling period, no blocks are missed because the program retrieves all blocks generated between intervals.
- Local Unix socket (this only works in Unix systems). The program has to be run with enough privileges to access the Unix socket from the blockchain node.

The program accumulates counters with the number of blocks that where proposed and sealed by each Validator since the `signers` program was started.
Optionally, you can specify a number of blocks in the past and the program will accumulate statistics for those blocks before beginning to display the current ones.

The help for the program is below (`signers help`):

```
NAME:
   signers - Monitoring of block signers activity for the Alastria RedT blockchain network

USAGE:
   signers [global options] command [command options]
         where 'nodeURL' is the address of the Quorum node.
         It supports both HTTP and WebSockets endpoints.
         By default it uses WebSockets and for HTTP you have to use the 'poll' subcommand.

VERSION:
   v0.1

AUTHOR:
   Jesus Ruiz <hesus.ruiz@gmail.com>

COMMANDS:
   monitor    monitor the signers activity via WebSockets events
   poll       monitor the signers activity via HTTP polling
   peers      display peers information
   logfilter  display filtered log information
   serve      run a web server to display signers behaviour in real time
   history    download blockchain headers into SQLite database, from current towards genesis
   historyfw  download blockchain headers into SQLite database, from newest stored towards current
   help, h    Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
```