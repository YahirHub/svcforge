package svcforge

import "errors"

// ServiceManager identifies the platform service manager selected by SvcForge.
type ServiceManager string

const (
	ServiceManagerSystemd ServiceManager = "systemd"
	ServiceManagerOpenRC  ServiceManager = "openrc"
	ServiceManagerWindows ServiceManager = "windows-scm"
)

// RestartPolicy controls whether the service manager should restart a crashed process.
type RestartPolicy string

const (
	RestartNever     RestartPolicy = "never"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartAlways    RestartPolicy = "always"
)

// ServiceState describes the currently installed service definition.
type ServiceState struct {
	Manager        ServiceManager
	DefinitionPath string
	Installed      bool
	Running        bool
	Enabled        bool
}

var ErrServiceManagerUnavailable = errors.New("no supported service manager is active")
