//go:build darwin

package processes

/*
#include <libproc.h>
*/
import "C"

import (
	"fmt"
	"syscall"
	"unsafe"
)

func listNative(uid int) ([]Process, error) {
	pids, err := listAllPIDs()
	if err != nil {
		return nil, err
	}

	procs := make([]Process, 0, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		info, err := fetchBSDInfo(pid)
		if err != nil {
			continue
		}
		if info == nil || int(info.pbi_uid) != uid {
			continue
		}
		cwd, err := processCWD(int(pid))
		if err != nil || cwd == "" {
			continue
		}
		command := C.GoString(&info.pbi_comm[0])
		command = sanitizeCommand(command, int(pid))
		procs = append(procs, Process{
			PID:     int(pid),
			PPID:    int(info.pbi_ppid),
			Command: command,
			CWD:     cwd,
		})
	}
	return procs, nil
}

func processCWD(pid int) (string, error) {
	var info C.struct_proc_vnodepathinfo
	size := C.int(unsafe.Sizeof(info))
	ret, err := C.proc_pidinfo(C.int(pid), C.PROC_PIDVNODEPATHINFO, 0, unsafe.Pointer(&info), size)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			switch errno {
			case syscall.EPERM, syscall.ESRCH:
				return "", nil
			}
		}
		return "", err
	}
	if ret <= 0 {
		return "", nil
	}
	if ret != size {
		return "", fmt.Errorf("short proc_pidinfo read: %d", ret)
	}
	return C.GoString(&info.pvi_cdir.vip_path[0]), nil
}

func listAllPIDs() ([]int32, error) {
	size := C.proc_listpids(C.PROC_ALL_PIDS, 0, nil, 0)
	if size <= 0 {
		return nil, fmt.Errorf("proc_listpids size %d", size)
	}
	count := int(size) / int(unsafe.Sizeof(C.pid_t(0)))
	if count == 0 {
		return nil, nil
	}
	buf := make([]C.pid_t, count)
	ret := C.proc_listpids(C.PROC_ALL_PIDS, 0, unsafe.Pointer(&buf[0]), size)
	if ret <= 0 {
		return nil, fmt.Errorf("proc_listpids returned %d", ret)
	}
	limit := int(ret) / int(unsafe.Sizeof(C.pid_t(0)))
	pids := make([]int32, 0, limit)
	for i := 0; i < limit; i++ {
		if buf[i] != 0 {
			pids = append(pids, int32(buf[i]))
		}
	}
	return pids, nil
}

func fetchBSDInfo(pid int32) (*C.struct_proc_bsdinfo, error) {
	var info C.struct_proc_bsdinfo
	size := C.int(unsafe.Sizeof(info))
	ret, err := C.proc_pidinfo(C.int(pid), C.PROC_PIDTBSDINFO, 0, unsafe.Pointer(&info), size)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			switch errno {
			case syscall.EPERM, syscall.ESRCH:
				return nil, nil
			}
		}
		return nil, err
	}
	if ret != size {
		return nil, nil
	}
	return &info, nil
}
