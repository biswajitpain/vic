Test 1-08 - Docker Logs
=======

#Purpose:
To verify that docker logs command is supported by VIC appliance

#References:
[1 - Docker Command Line Reference](https://docs.docker.com/engine/reference/commandline/logs/)

#Environment:
This test requires that a vSphere server is running and available

#Test Steps:
1. Deploy VIC appliance to vSphere server
2. Issue docker create busybox /bin/sh -c 'seq 1 5000' to the VIC appliance
3. Issue docker start <containerID> to the VIC appliance
4. Issue docker logs <containerID> to the VIC appliance
5. Issue docker logs --tail=all <containerID> to the VIC appliance
6. Issue docker logs --tail=200 <containerID> to the VIC appliance
7. Issue docker logs --tail=0 <containerID> to the VIC appliance
8. Issue docker create -t busybox /bin/sh -c 'for i in $(seq 1 5) ; do sleep 1 && echo line $i; done'
9. Issue docker start <containerID> to the VIC appliance
10. Issue docker logs --follow <containerID> to the VIC appliance
11. Issue docker create busybox /bin/sh -c 'trap "seq 11 20; exit" HUP; seq 1 10; while true; do sleep 1; done'
12. Issue docker start <containerID> to the VIC appliance
13. Issue docker logs <containerID> to the VIC appliance, waiting for the first 10 lines
14. Issue docker kill -s HUP <containerID> to the VIC appliance, generating the next 10 lines
15. Issue docker logs --tail=5 --follow <containerID> to the VIC appliance
16. Issue docker logs --since=1s <containerID> to the VIC appliance
17. Issue docker logs --timestamps <containerID> to the VIC appliance
18. Issue docker logs
19. Issue docker logs fakeContainer

#Expected Outcome:
* Steps 2-15 should all complete without error
* Step 6 should output 200 lines
* Step 7 should output 0 lines
* Step 10 should have last line be
```
line 5
```
* Step 13 should output 10 lines
* Step 15 should output 15 lines
* Step 16 should output 3 lines
* Step 17 and 18 should result in an error with the following message:
```
Error: vSphere Integrated Containers does not yet support timestampped logs.
```
* Step 18 should output all lines
* Step 20 should result in an error with the following message:
```
Error: No such container: fakeContainer
```

#Possible Problems:
None
