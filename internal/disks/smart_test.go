package disks

import (
	"errors"
	"testing"
)

const ironWolfJSON = `{
  "json_format_version": [1, 0],
  "smartctl": {"version": [7, 5], "exit_status": 0, "messages": []},
  "device": {"name": "/dev/sda", "info_name": "/dev/sda [SAT]", "type": "sat", "protocol": "ATA"},
  "model_family": "Seagate IronWolf",
  "model_name": "ST8000VN004-3CP101",
  "serial_number": "WWZ5ZGFL",
  "firmware_version": "SC60",
  "user_capacity": {"blocks": 15628053168, "bytes": 8001563222016},
  "rotation_rate": 7200,
  "smart_status": {"passed": true},
  "ata_smart_attributes": {
    "table": [
      {"id": 5, "name": "Reallocated_Sector_Ct", "value": 100, "worst": 100, "raw": {"value": 0, "string": "0"}},
      {"id": 9, "name": "Power_On_Hours", "value": 78, "worst": 78, "raw": {"value": 19876, "string": "19876"}},
      {"id": 190, "name": "Airflow_Temperature_Cel", "value": 62, "worst": 48, "raw": {"value": 38, "string": "38"}},
      {"id": 194, "name": "Temperature_Celsius", "value": 38, "worst": 52, "raw": {"value": 38, "string": "38"}},
      {"id": 197, "name": "Current_Pending_Sector", "value": 100, "worst": 100, "raw": {"value": 0, "string": "0"}},
      {"id": 198, "name": "Offline_Uncorrectable", "value": 100, "worst": 100, "raw": {"value": 0, "string": "0"}},
      {"id": 199, "name": "UDMA_CRC_Error_Count", "value": 200, "worst": 200, "raw": {"value": 3, "string": "3"}}
    ]
  },
  "power_on_time": {"hours": 19876},
  "power_cycle_count": 42,
  "temperature": {"current": 38}
}`

const failingJSON = `{
  "smartctl": {"exit_status": 8, "messages": []},
  "device": {"name": "/dev/sdb", "type": "sat", "protocol": "ATA"},
  "model_name": "WDC WD40EFRX",
  "serial_number": "WD-ABC",
  "smart_status": {"passed": false},
  "ata_smart_attributes": {"table": [
    {"id": 5, "name": "Reallocated_Sector_Ct", "value": 1, "worst": 1, "raw": {"value": 1532, "string": "1532"}},
    {"id": 197, "name": "Current_Pending_Sector", "value": 100, "worst": 100, "raw": {"value": 8, "string": "8"}}
  ]},
  "temperature": {"current": 61}
}`

const standbyJSON = `{
  "smartctl": {"exit_status": 2, "messages": [{"string": "Device is in STANDBY mode, exit(2)", "severity": "error"}]},
  "device": {"name": "/dev/sdc", "type": "sat", "protocol": "ATA"}
}`

const nvmeJSON = `{
  "smartctl": {"exit_status": 0},
  "device": {"name": "/dev/nvme0", "type": "nvme", "protocol": "NVMe"},
  "model_name": "Samsung SSD 980 PRO 1TB",
  "serial_number": "S5GXNX0T",
  "firmware_version": "5B2QGXA7",
  "user_capacity": {"bytes": 1000204886016},
  "smart_status": {"passed": true},
  "nvme_smart_health_information_log": {
    "critical_warning": 0, "temperature": 41, "percentage_used": 3,
    "power_cycles": 120, "power_on_hours": 5120, "media_errors": 0
  },
  "temperature": {"current": 41},
  "power_on_time": {"hours": 5120}
}`

func TestParseSmartctlIronWolf(t *testing.T) {
	d, err := ParseSmartctl([]byte(ironWolfJSON))
	if err != nil {
		t.Fatal(err)
	}
	if d.Device != "/dev/sda" || d.Model != "ST8000VN004-3CP101" || d.Serial != "WWZ5ZGFL" {
		t.Fatalf("identity %+v", d)
	}
	if d.Temperature != 38 || d.PowerOnHours != 19876 || d.PowerCycles != 42 {
		t.Fatalf("counters %+v", d)
	}
	if d.CRCErrors != 3 || d.Reallocated != 0 || d.Pending != 0 {
		t.Fatalf("attributes %+v", d)
	}
	if !d.SmartKnown || !d.SmartPassed || !d.Healthy() {
		t.Fatalf("health %+v", d)
	}
	if d.CapacityBytes != 8001563222016 || d.RotationRPM != 7200 {
		t.Fatalf("capacity %+v", d)
	}
}

func TestParseSmartctlFailingDrive(t *testing.T) {
	d, err := ParseSmartctl([]byte(failingJSON))
	if err != nil {
		t.Fatal(err)
	}
	if d.SmartPassed || d.Healthy() {
		t.Fatalf("expected unhealthy %+v", d)
	}
	if d.Reallocated != 1532 || d.Pending != 8 || d.Temperature != 61 {
		t.Fatalf("attributes %+v", d)
	}
}

func TestParseSmartctlStandby(t *testing.T) {
	d, err := ParseSmartctl([]byte(standbyJSON))
	if !errors.Is(err, ErrStandby) {
		t.Fatalf("expected ErrStandby, got %v", err)
	}
	if d.Device != "/dev/sdc" || !d.Standby {
		t.Fatalf("standby %+v", d)
	}
}

func TestParseSmartctlNVMe(t *testing.T) {
	d, err := ParseSmartctl([]byte(nvmeJSON))
	if err != nil {
		t.Fatal(err)
	}
	if d.Transport != "NVMe" || d.Temperature != 41 || d.PowerOnHours != 5120 || d.PercentUsed != 3 {
		t.Fatalf("nvme %+v", d)
	}
	if !d.Healthy() {
		t.Fatal("expected healthy nvme")
	}
}

func TestParseSmartctlRejectsGarbage(t *testing.T) {
	if _, err := ParseSmartctl([]byte("not json")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseSmartctl([]byte(`{"smartctl":{}}`)); err == nil {
		t.Fatal("expected error for missing device")
	}
}
