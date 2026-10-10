//go:build windows

package sys

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobAccounting is JOBOBJECT_BASIC_ACCOUNTING_INFORMATION (not exported by x/sys/windows).
type jobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

var (
	jobsMu sync.Mutex
	jobs   = map[int]windows.Handle{}
)

// Detach starts the child in a new process group (Ctrl+C in the panel does not reach it) so it can
// be assigned to a Job Object right after Start (see Attach).
func Detach(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}

// Attach puts a started process in a Job Object that kills every descendant (npm → bash → tsc +
// node) when it closes. Call right after c.Start(). Grandchildren spawned before the assignment
// are rare (npm takes >100ms to spawn) and still caught by KillTree's taskkill /T.
func Attach(c *exec.Cmd) {
	if c == nil || c.Process == nil {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return
	}
	jobsMu.Lock()
	jobs[c.Process.Pid] = job
	jobsMu.Unlock()
}

// KillTree stops a started command and every descendant.
func KillTree(c *exec.Cmd) {
	if c == nil || c.Process == nil {
		return
	}
	pid := c.Process.Pid
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	jobsMu.Lock()
	job, ok := jobs[pid]
	delete(jobs, pid)
	jobsMu.Unlock()
	if ok {
		_ = windows.TerminateJobObject(job, 1)
		windows.CloseHandle(job)
	}
	_ = c.Process.Kill()
}

// KillPID stops one process tree by pid.
func KillPID(pid int) {
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

// KillGroup stops what a started process left behind. With the Job Object of this panel the whole
// tree goes, even after the root exited; without one (a previous, crashed panel) only a live root
// pid is killed with its tree — never a dead pid, which Windows may already have reused.
func KillGroup(pid int) {
	jobsMu.Lock()
	job, ok := jobs[pid]
	delete(jobs, pid)
	jobsMu.Unlock()
	if ok {
		_ = windows.TerminateJobObject(job, 1)
		windows.CloseHandle(job)
		return
	}
	if Alive(pid) {
		KillPID(pid)
	}
}

// GroupAlive reports whether the job of pid (or the root process itself) still runs.
func GroupAlive(pid int) bool {
	jobsMu.Lock()
	job, ok := jobs[pid]
	jobsMu.Unlock()
	if ok {
		var info jobAccounting
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err == nil {
			return info.ActiveProcesses > 0
		}
	}
	return Alive(pid)
}

// Alive reports whether a process exists.
func Alive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

// ProcessName returns the image name of a pid ("" when unknown).
func ProcessName(pid int) string { return processName(pid) }

func processName(pid int) string {
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.Index(line, "\",\""); i > 1 && strings.HasPrefix(line, "\"") {
		return line[1:i]
	}
	return ""
}
