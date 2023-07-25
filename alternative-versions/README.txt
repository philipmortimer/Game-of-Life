Within this folder are our coursework submissions. This contains
versions of our parrallel and distributed systems discussed in the reports but not included in the
uploaded zip file. All these versions pass the relevant tests. The only exception to this is the pure
memory sharing implementation. This doesn't pass all the University provided tests as they have been
written based on the assumption that channels are used for key pressses and events. We have rewritten 
TestGol to work with our solution (so please run "go test -v -run=TestGol"). Additionally, "go run ."
works and is well worth viewing due to how much faster it is than the corresponding channel based
implementation.

In order to get "Parallel AWS nodes" (or even our actual submission for the distributed system) locally,
execute these commands in one terminal window per command. Execute the commands in the order you see them.
----------------
go run server/server.go -port=8038
go run server/server.go -port=8039
go run server/server.go -port=8040
go run broker/broker.go -port=8030 -serverAddressesList="127.0.0.1:8038,127.0.0.1:8039,127.0.0.1:8040"
----------------
After these commands you may wish to (in a new terminal) run a command like "go run ." or "go test -v -run=TestGol/-1$"

In order to run "SDL Live View" locally, execute these commands in one terminal window per command.
Exectute the commands in the order you see them.
--------------------------
go run server/server.go -port=8038
go run server/server.go -port=8039
go run server/server.go -port=8040
go run broker/broker.go -port=8031 -serverAddressesList="127.0.0.1:8038,127.0.0.1:8039,127.0.0.1:8040"
-----------------------------
After these commands you may wish to (in a new terminal) run a command like "go run ." or "go test -v -run=TestGol/-1$"

Please enjoy!
