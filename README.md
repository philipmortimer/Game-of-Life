# Game-of-Life
 An implementation of Conway's Game of Life using parallel and distributed computation. The 'board' used for the game is divided into chunks which are handled by different threads to allow for concurrent calculations. Low level synchronisation methods such as semaphores and mutex locks are used, in combination with more high level abstractions such as channels (used in the Go programming language). In the distributed section, board sections are sent to different computer nodes to be updated. A broker is used to co-ordinate this process. We used AWS to test the performance of our distributed solution. This work was completed as part of a coursework by myself and a coursemate and recieved a first class grade.
Here is a video of the parralel solution in action:

https://github.com/philipmortimer/Game-of-Life/assets/64362945/525b025f-0497-42e4-8c65-c3f646c1710a

To run the parallel version, change directory to "parallel" and then simply execute "go run .". To run unit tests, run "go test -v". Go version: 1.13.

To run the distributed version, change directory to "distributed". You can then run server nodes and a broker node by executing the following commands in order. Execute one command per terminal window. This method can be used to achieve distrbuted computation using nodes connected via the internet (e.g. AWS) or just run locally.
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
go run server/server.go -port=8038
go run server/server.go -port=8039
go run server/server.go -port=8040
go run broker/broker.go -port=8030 -serverAddressesList="127.0.0.1:8038,127.0.0.1:8039,127.0.0.1:8040"
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
After these commands you may wish to (in a new terminal) run a command like "go run ." or "go test -v -run=TestGol/-1$".

The [parallel](parallel) directory contains the parallelised solution. Similarly, the [distributed](distributed) directory contains the distributed solution. We produced a short report discussing our solution: [report](report.pdf). We also produced a bunch of [alternative versions](Alternative Versions) which used different techniques and were submitted for certain coursework extensions. The [coursework brief](coursework-brief/README.md) outlines the project specification.

 



