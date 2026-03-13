# usage: target_factory <ip>

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <ip>"
    exit 1
fi

dut_ip=$1

pass="cheesebread"
sshpass -p $pass ssh -o StrictHostKeyChecking=no root@$dut_ip \
    "rm -rf /data/me"