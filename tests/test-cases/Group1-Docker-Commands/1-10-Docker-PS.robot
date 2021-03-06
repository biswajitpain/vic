*** Settings ***
Documentation  Test 1-10 - Docker PS
Resource  ../../resources/Util.robot
Suite Setup  Install VIC Appliance To Test Server
Suite Teardown  Cleanup VIC Appliance On Test Server

*** Keywords ***
Assert VM Power State
    [Arguments]  ${name}  ${state}
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run And Return Rc And Output  govc vm.info -json %{VCH-NAME}/${name}-* | jq -r .VirtualMachines[].Runtime.PowerState
    Run Keyword If  '%{HOST_TYPE}' == 'VC'  Should Be Equal As Integers  ${rc}  0
    Run Keyword If  '%{HOST_TYPE}' == 'VC'  Should Be Equal  ${output}  ${state}
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run And Return Rc And Output  govc vm.info -json ${name}-* | jq -r .VirtualMachines[].Runtime.PowerState
    Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Should Be Equal As Integers  ${rc}  0
    Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Should Be Equal  ${output}  ${state}

Create several containers
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} pull busybox
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${container2}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create busybox ls
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} start ${container2}
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${container1}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create busybox /bin/top
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} start ${container1}
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${container3}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create busybox dmesg
    Should Be Equal As Integers  ${rc}  0
    Wait Until VM Powers Off  *-${container2}

Assert Number Of Containers
    [Arguments]  ${num}  ${type}=-q
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps ${type}
    Should Be Equal As Integers  ${rc}  0
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  ${num}

*** Test Cases ***
Empty docker ps command
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  CONTAINER ID
    Should Contain  ${output}  IMAGE
    Should Contain  ${output}  COMMAND
    Should Contain  ${output}  CREATED
    Should Contain  ${output}  STATUS
    Should Contain  ${output}  PORTS
    Should Contain  ${output}  NAMES
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  1

Docker ps only running containers
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps
    Should Be Equal As Integers  ${rc}  0
    ${output}=  Split To Lines  ${output}
    ${len}=  Get Length  ${output}
    Create several containers
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  /bin/top
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  ${len+1}

Docker ps all containers
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -a
    Should Be Equal As Integers  ${rc}  0
    ${output}=  Split To Lines  ${output}
    ${len}=  Get Length  ${output}
    Create several containers
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -a
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  /bin/top
    Should Contain  ${output}  dmesg
    Should Contain  ${output}  ls
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  ${len+3}

Docker ps powerOn container OOB
    ${rc}  ${container}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create --name jojo busybox /bin/top
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -q
    Should Be Equal As Integers  ${rc}  0
    ${output}=  Split To Lines  ${output}
    ${len}=  Get Length  ${output}

    Power On VM OOB  jojo*

    Wait Until Keyword Succeeds  10x  6s  Assert Number Of Containers  ${len+1}

Docker ps powerOff container OOB
    ${rc}  ${container}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create --name koko busybox /bin/top
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} start koko
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -q
    Should Be Equal As Integers  ${rc}  0
    ${output}=  Split To Lines  ${output}
    ${len}=  Get Length  ${output}

    Power Off VM OOB  koko*

    Wait Until Keyword Succeeds  10x  6s  Assert Number Of Containers  ${len-1}

Docker ps ports output
    ${rc}  ${container}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create -p 8000:80 -p 8443:443 nginx
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -a
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  :8000->80/tcp
    Should Contain  ${output}  :8443->443/tcp

    ${rc}  ${container}=  Run And Return Rc And Output  docker %{VCH-PARAMS} run -d -p 6379 redis:alpine
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  ->6379/tcp

Docker ps Remove container OOB
    ${rc}  ${container}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create --name lolo busybox /bin/top
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} start lolo
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} stop lolo
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -aq
    Should Be Equal As Integers  ${rc}  0
    ${output}=  Split To Lines  ${output}
    ${len}=  Get Length  ${output}
    # Remove container VM out-of-band
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'VC'  Run And Return Rc And Output  govc vm.destroy %{VCH-NAME}/"lolo*"
    Run Keyword If  '%{HOST_TYPE}' == 'VC'  Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Run And Return Rc And Output  govc vm.destroy "lolo*"
    Run Keyword If  '%{HOST_TYPE}' == 'ESXi'  Should Be Equal As Integers  ${rc}  0
    Wait Until VM Is Destroyed  "lolo*"
    Wait Until Keyword Succeeds  10x  6s  Assert Number Of Containers  ${len-1}  -aq
    ${rc}  ${output}=  Run And Return Rc And Output  govc datastore.ls | grep lolo- | xargs -n1 govc datastore.rm
    Should Be Equal As Integers  ${rc}  0

Docker ps last container
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -l
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  redis
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  2

Docker ps two containers
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -n=2
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  redis
    Should Contain  ${output}  nginx
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  3

Docker ps last container with size
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -ls
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  SIZE
    Should Contain  ${output}  redis
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  2

Docker ps all containers with only IDs
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -aq
    ${output}=  Split To Lines  ${output}
    ${len}=  Get Length  ${output}
    Create several containers
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -aq
    Should Be Equal As Integers  ${rc}  0
    Should Not Contain  ${output}  CONTAINER ID
    Should Not Contain  ${output}  /bin/top
    Should Not Contain  ${output}  dmesg
    Should Not Contain  ${output}  ls
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  ${len+3}

Docker ps with status filter
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -f status=created
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  nginx
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  5

Docker ps with label and name filter
    ${rc}  ${container}=  Run And Return Rc And Output  docker %{VCH-PARAMS} create --name abe --label prod busybox /bin/top
    Should Be Equal As Integers  ${rc}  0
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -a -f label=prod
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  busybox
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  2
    ${rc}  ${output}=  Run And Return Rc And Output  docker %{VCH-PARAMS} ps -a -f name=abe
    Should Be Equal As Integers  ${rc}  0
    Should Contain  ${output}  busybox
    ${output}=  Split To Lines  ${output}
    Length Should Be  ${output}  2