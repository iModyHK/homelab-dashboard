package disks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Disk struct {
	Device        string `json:"device"`
	Model         string `json:"model"`
	Serial        string `json:"serial"`
	Firmware      string `json:"firmware"`
	CapacityBytes uint64 `json:"capacityBytes"`
	RotationRPM   int    `json:"rotationRpm"`
	Transport     string `json:"transport"`
	Temperature   int    `json:"temperature"`
	PowerOnHours  int64  `json:"powerOnHours"`
	PowerCycles   int64  `json:"powerCycles"`
	Reallocated   int64  `json:"reallocated"`
	Pending       int64  `json:"pending"`
	Uncorrectable int64  `json:"uncorrectable"`
	CRCErrors     int64  `json:"crcErrors"`
	SmartPassed   bool   `json:"smartPassed"`
	SmartKnown    bool   `json:"smartKnown"`
	Standby       bool   `json:"standby"`
	PercentUsed   int    `json:"percentUsed"`
}

type smartctlOutput struct {
	Device struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	} `json:"device"`
	Smartctl struct {
		ExitStatus int `json:"exit_status"`
		Messages   []struct {
			String   string `json:"string"`
			Severity string `json:"severity"`
		} `json:"messages"`
	} `json:"smartctl"`
	ModelFamily     string `json:"model_family"`
	ModelName       string `json:"model_name"`
	SerialNumber    string `json:"serial_number"`
	FirmwareVersion string `json:"firmware_version"`
	UserCapacity    struct {
		Bytes uint64 `json:"bytes"`
	} `json:"user_capacity"`
	RotationRate int `json:"rotation_rate"`
	SmartStatus  *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
	PowerCycleCount    int64 `json:"power_cycle_count"`
	AtaSmartAttributes struct {
		Table []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Value int    `json:"value"`
			Worst int    `json:"worst"`
			Raw   struct {
				Value  int64  `json:"value"`
				String string `json:"string"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
	NvmeSmartHealthInformationLog *struct {
		Temperature     int   `json:"temperature"`
		PercentageUsed  int   `json:"percentage_used"`
		PowerOnHours    int64 `json:"power_on_hours"`
		PowerCycles     int64 `json:"power_cycles"`
		MediaErrors     int64 `json:"media_errors"`
		CriticalWarning int   `json:"critical_warning"`
	} `json:"nvme_smart_health_information_log"`
}

var ErrStandby = errors.New("device is in standby")

func ParseSmartctl(raw []byte) (Disk, error) {
	var out smartctlOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return Disk{}, fmt.Errorf("smartctl json: %w", err)
	}
	d := Disk{
		Device:        out.Device.Name,
		Model:         firstNonEmpty(out.ModelName, out.ModelFamily),
		Serial:        out.SerialNumber,
		Firmware:      out.FirmwareVersion,
		CapacityBytes: out.UserCapacity.Bytes,
		RotationRPM:   out.RotationRate,
		Transport:     firstNonEmpty(out.Device.Protocol, out.Device.Type),
		Temperature:   out.Temperature.Current,
		PowerOnHours:  out.PowerOnTime.Hours,
		PowerCycles:   out.PowerCycleCount,
	}
	if out.SmartStatus != nil {
		d.SmartKnown = true
		d.SmartPassed = out.SmartStatus.Passed
	}
	for _, m := range out.Smartctl.Messages {
		if strings.Contains(strings.ToLower(m.String), "standby") {
			d.Standby = true
		}
	}
	if out.Smartctl.ExitStatus == 2 && d.Model == "" {
		d.Standby = true
	}
	for _, attr := range out.AtaSmartAttributes.Table {
		switch attr.ID {
		case 5:
			d.Reallocated = attr.Raw.Value
		case 197:
			d.Pending = attr.Raw.Value
		case 198:
			d.Uncorrectable = attr.Raw.Value
		case 199:
			d.CRCErrors = attr.Raw.Value
		case 194, 190:
			if d.Temperature == 0 {
				d.Temperature = int(attr.Raw.Value & 0xFF)
			}
		case 9:
			if d.PowerOnHours == 0 {
				d.PowerOnHours = attr.Raw.Value & 0xFFFFFFFF
			}
		}
	}
	if nvme := out.NvmeSmartHealthInformationLog; nvme != nil {
		if d.Temperature == 0 {
			d.Temperature = nvme.Temperature
		}
		if d.PowerOnHours == 0 {
			d.PowerOnHours = nvme.PowerOnHours
		}
		if d.PowerCycles == 0 {
			d.PowerCycles = nvme.PowerCycles
		}
		d.Uncorrectable = nvme.MediaErrors
		d.PercentUsed = nvme.PercentageUsed
		if nvme.CriticalWarning != 0 {
			d.SmartKnown = true
			d.SmartPassed = false
		}
	}
	if d.Device == "" {
		return d, errors.New("smartctl output has no device name")
	}
	if d.Standby && d.Model == "" {
		return d, ErrStandby
	}
	return d, nil
}

func (d Disk) Healthy() bool {
	if d.SmartKnown && !d.SmartPassed {
		return false
	}
	return d.Reallocated == 0 && d.Pending == 0 && d.Uncorrectable == 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
