package ipc

import (
	"strconv"
	"strings"

	"github.com/YLonely/cer-manager/utils"
)

const (
	kernelSem              string = "kernel/sem"
	kernelMsgMax           string = "kernel/msgmax"
	kernelMsgMnb           string = "kernel/msgmnb"
	kernelMsgMni           string = "kernel/msgmni"
	kernelAutoMsgMni       string = "kernel/auto_msgmni"
	kernelShmMax           string = "kernel/shmmax"
	kernelShmAll           string = "kernel/shmall"
	kernelShmMni           string = "kernel/shmmni"
	kernelShmRmidForced    string = "kernel/shm_rmid_forced"
	kernelMsgNextID        string = "kernel/msg_next_id"
	kernelSemNextID        string = "kernel/sem_next_id"
	kernelShmNextID        string = "kernel/shm_next_id"
	fsQueuesMax            string = "fs/mqueue/queues_max"
	fsQueuesMsgMax         string = "fs/mqueue/msg_max"
	fsQueuesMsgSizeMax     string = "fs/mqueue/msgsize_max"
	fsQueuesMsgDefault     string = "fs/mqueue/msg_default"
	fsQueuesMsgSizeDefault string = "fs/mqueue/msgsize_default"
)

var dumpFileNamePrefixes = []string{"ipcns-sem-", "ipcns-msg-", "ipcns-shm-", "ipcns-var-"}

var sources = map[string]utils.Source{
	"SemCtls": func() (interface{}, error) {
		str, err := utils.SysCtlRead(kernelSem)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(str, "\t")
		ret := make([]uint32, 0, len(parts))
		for _, part := range parts {
			v, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return nil, err
			}
			ret = append(ret, uint32(v))
		}
		return ret, nil
	},
	"MsgCtlmax":        readSysUint32(kernelMsgMax),
	"MsgCtlmnb":        readSysUint32(kernelMsgMnb),
	"MsgCtlmni":        readSysUint32(kernelMsgMni),
	"AutoMsgmni":       readSysUint32(kernelAutoMsgMni),
	"ShmCtlmax":        readSysUint64(kernelShmMax),
	"ShmCtlall":        readSysUint64(kernelShmAll),
	"ShmCtlmni":        readSysUint32(kernelShmMni),
	"ShmRmidForced":    readSysUint32(kernelShmRmidForced),
	"MqQueuesMax":      readSysUint32(fsQueuesMax),
	"MqMsgMax":         readSysUint32(fsQueuesMsgMax),
	"MqMsgsizeMax":     readSysUint32(fsQueuesMsgSizeMax),
	"MqMsgDefault":     readSysUint32(fsQueuesMsgDefault),
	"MqMsgsizeDefault": readSysUint32(fsQueuesMsgSizeDefault),
	"MsgNextId":        readSysUint32(kernelMsgNextID),
	"SemNextId":        readSysUint32(kernelSemNextID),
	"ShmNextId":        readSysUint32(kernelShmNextID),
}

var targets = map[string]utils.Target{
	"SemCtls": func(v interface{}) error {
		if v == nil {
			return nil
		}
		nums := v.([]uint32)
		if nums == nil {
			return nil
		}
		parts := make([]string, 0, len(nums))
		for _, num := range nums {
			parts = append(parts, strconv.FormatUint(uint64(num), 10))
		}
		str := strings.Join(parts, "\t")
		if err := utils.SysCtlWrite(kernelSem, str); err != nil {
			return err
		}
		return nil
	},
	"MsgCtlmax":        writeSysUint32(kernelMsgMax),
	"MsgCtlmnb":        writeSysUint32(kernelMsgMnb),
	"MsgCtlmni":        writeSysUint32(kernelMsgMni),
	"AutoMsgmni":       writeSysUint32(kernelAutoMsgMni),
	"ShmCtlmax":        writeSysUint64(kernelShmMax),
	"ShmCtlall":        writeSysUint64(kernelShmAll),
	"ShmCtlmni":        writeSysUint32(kernelShmMni),
	"ShmRmidForced":    writeSysUint32(kernelShmRmidForced),
	"MqQueuesMax":      writeSysUint32(fsQueuesMax),
	"MqMsgMax":         writeSysUint32(fsQueuesMsgMax),
	"MqMsgsizeMax":     writeSysUint32(fsQueuesMsgSizeMax),
	"MqMsgDefault":     writeSysUint32(fsQueuesMsgDefault),
	"MqMsgsizeDefault": writeSysUint32(fsQueuesMsgSizeDefault),
	"MsgNextId":        writeSysUint32(kernelMsgNextID),
	"SemNextId":        writeSysUint32(kernelSemNextID),
	"ShmNextId":        writeSysUint32(kernelShmNextID),
}

func readSysUint64(name string) utils.Source {
	return func() (interface{}, error) {
		str, err := utils.SysCtlRead(name)
		if err != nil {
			return nil, err
		}
		if str == "" || str[0] == '-' {
			return nil, nil
		}
		v, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
}

func readSysUint32(name string) utils.Source {
	return func() (interface{}, error) {
		i, err := readSysUint64(name)()
		if err != nil || i == nil {
			return nil, err
		}
		v := *(i.(*uint64))
		vv := uint32(v)
		return &vv, nil
	}
}

func writeSysUint64(name string) utils.Target {
	return func(v interface{}) error {
		if v == nil {
			return nil
		}
		ptr := v.(*uint64)
		if ptr == nil {
			return nil
		}
		str := strconv.FormatUint(*ptr, 10)
		if err := utils.SysCtlWrite(name, str); err != nil {
			return err
		}
		return nil
	}
}

func writeSysUint32(name string) utils.Target {
	return func(v interface{}) error {
		if v == nil {
			return nil
		}
		ptr := v.(*uint32)
		if ptr == nil {
			return nil
		}
		num := uint64(*ptr)
		return writeSysUint64(name)(&num)
	}
}
