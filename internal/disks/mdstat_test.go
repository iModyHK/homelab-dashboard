package disks

import "testing"

const terraMasterMdstat = `Personalities : [linear] [raid0] [raid1] [raid10] [raid6] [raid5] [raid4] [multipath] [faulty]
md1 : active raid5 sdc5[0] sdb5[4] sda5[3] sdf5[2] sdd5[1]
      23443081216 blocks super 1.2 level 5, 512k chunk, algorithm 2 [5/5] [UUUUU]

md0 : active raid5 sdza4[6] sdb4[5] sda4[4] sdf4[3] sdd4[2] sdc4[1]
      9713497920 blocks super 1.2 level 5, 64k chunk, algorithm 2 [6/6] [UUUUUU]

md8 : active raid1 sdza3[0]
      1997824 blocks super 1.2 [1/1] [U]
      bitmap: 0/1 pages [0KB], 65536KB chunk

md9 : active raid1 sdza2[0]
      7995392 blocks super 1.2 [1/1] [U]
      bitmap: 0/1 pages [0KB], 65536KB chunk

unused devices: <none>
`

const degradedMdstat = `Personalities : [raid1] [raid5]
md0 : active raid5 sdb1[1] sdc1[2] sda1[0](F)
      1953260544 blocks level 5, 64k chunk, algorithm 2 [3/2] [_UU]

md1 : active raid1 sdd1[2] sde1[1]
      976630464 blocks [2/1] [_U]
      [==>..................]  recovery = 12.5% (122078720/976630464) finish=85.3min speed=166943K/sec

unused devices: <none>
`

func TestParseMdstatTerraMaster(t *testing.T) {
	arrays := ParseMdstat(terraMasterMdstat)
	if len(arrays) != 4 {
		t.Fatalf("expected 4 arrays, got %d", len(arrays))
	}
	byName := map[string]Array{}
	for _, a := range arrays {
		byName[a.Name] = a
	}

	md1 := byName["md1"]
	if md1.Level != "raid5" || md1.State != "clean" || md1.Degraded {
		t.Fatalf("md1 %+v", md1)
	}
	if md1.SlotsTotal != 5 || md1.SlotsActive != 5 || len(md1.Members) != 5 {
		t.Fatalf("md1 slots %+v", md1)
	}
	if md1.Blocks != 23443081216 {
		t.Fatalf("md1 blocks %d", md1.Blocks)
	}
	if md1.Members[0].Device != "sdc5" || md1.Members[0].Slot != 0 {
		t.Fatalf("md1 member %+v", md1.Members[0])
	}

	md0 := byName["md0"]
	if len(md0.Members) != 6 || md0.Members[0].Device != "sdza4" || md0.Members[0].Slot != 6 {
		t.Fatalf("md0 members %+v", md0.Members)
	}

	md9 := byName["md9"]
	if md9.Level != "raid1" || md9.SlotsTotal != 1 || !md9.Healthy() {
		t.Fatalf("md9 %+v", md9)
	}
}

func TestParseMdstatDegradedAndRecovering(t *testing.T) {
	arrays := ParseMdstat(degradedMdstat)
	if len(arrays) != 2 {
		t.Fatalf("expected 2 arrays, got %d", len(arrays))
	}
	md0 := arrays[0]
	if md0.State != "degraded" || !md0.Degraded || md0.SlotsActive != 2 {
		t.Fatalf("md0 %+v", md0)
	}
	var faulty int
	for _, m := range md0.Members {
		if m.Faulty {
			faulty++
			if m.Device != "sda1" {
				t.Fatalf("wrong faulty member %+v", m)
			}
		}
	}
	if faulty != 1 {
		t.Fatalf("expected one faulty member, got %d", faulty)
	}

	md1 := arrays[1]
	if md1.State != "recovery" || md1.SyncPercent != 12.5 || md1.SyncFinishIn != "85.3 min" {
		t.Fatalf("md1 %+v", md1)
	}
	if md1.Healthy() {
		t.Fatal("recovering array must not be healthy")
	}
}

func TestParseMdstatEmpty(t *testing.T) {
	if got := ParseMdstat("Personalities : \nunused devices: <none>\n"); len(got) != 0 {
		t.Fatalf("expected no arrays, got %+v", got)
	}
}
