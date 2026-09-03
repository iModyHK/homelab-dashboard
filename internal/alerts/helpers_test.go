package alerts

import "github.com/iModyHK/homelab-dashboard/internal/disks"

func degradedArray() disks.Array {
	return disks.ParseMdstat(`Personalities : [raid5]
md0 : active raid5 sdb1[1] sdc1[2] sda1[0](F)
      1953260544 blocks level 5, 64k chunk, algorithm 2 [3/2] [_UU]

unused devices: <none>
`)[0]
}
