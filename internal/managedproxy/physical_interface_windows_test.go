package managedproxy

import (
	"context"
	"reflect"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsUplinkUsesPhysicalUplinkWithoutIPv4Binding(t *testing.T) {
	// An external switch keeps the physical NIC's LUID even after moving
	// the host's IPv4 binding to its management interface.
	hardware := windows.IpAdapterAddresses{Luid: 101, OperStatus: windows.IfOperStatusUp}
	mac := [6]byte{0x00, 0x15, 0x5d, 1, 2, 3}
	physical, err := readWindowsAdapterInfo(&hardware, func(row *windows.MibIfRow2) error {
		if row.InterfaceLuid != hardware.Luid {
			t.Fatal("physical adapter must be looked up by its LUID")
		}
		row.InterfaceIndex = 4
		row.OperStatus = windows.IfOperStatusUp
		row.InterfaceAndOperStatusFlags = windowsHardwareInterface
		row.PhysicalAddressLength = uint32(len(mac))
		copy(row.PhysicalAddress[:], mac[:])
		return nil
	})
	if err != nil || !physical.hardware || physical.ipv4 {
		t.Fatalf("hardware uplink not recognized: hardware=%v ipv4=%v err=%v", physical.hardware, physical.ipv4, err)
	}
	management := windowsAdapterInfo{index: 12, name: "vEthernet (External)", metric: 15, ipv4: true, mac: mac[:]}
	want := map[uint32]windowsPhysicalAdapter{12: {name: management.name, metric: management.metric}}
	if got := windowsUplinkAdapters([]windowsAdapterInfo{physical, management}); !reflect.DeepEqual(got, want) {
		t.Fatalf("physical uplink with a bridged IPv4 route = %+v, want %+v", got, want)
	}
}

func TestSelectWindowsPhysicalInterface(t *testing.T) {
	adapters := map[uint32]windowsPhysicalAdapter{
		3:  {name: "Ethernet", metric: 30},
		28: {name: "Wi-Fi", metric: 10},
	}
	route := func(index, metric uint32) windows.MibIpForwardRow2 {
		r := windows.MibIpForwardRow2{InterfaceIndex: index, Metric: metric}
		r.DestinationPrefix.Prefix.Family = windows.AF_INET
		*(*windows.RawSockaddrInet4)(unsafe.Pointer(&r.NextHop)) = windows.RawSockaddrInet4{
			Family: windows.AF_INET,
			Addr:   [4]byte{192, 168, 1, 1},
		}
		return r
	}

	virtual := route(9, 0)
	nonDefault := route(3, 0)
	nonDefault.DestinationPrefix.PrefixLength = 24
	onLink := route(28, 0)
	(*windows.RawSockaddrInet4)(unsafe.Pointer(&onLink.NextHop)).Addr = [4]byte{}

	if got := selectWindowsPhysicalInterface(adapters, []windows.MibIpForwardRow2{
		virtual, nonDefault, onLink, route(3, 1), route(28, 20),
	}); got != "Wi-Fi" {
		t.Fatalf("selected %q, want Wi-Fi with lower combined metric", got)
	}
	if got := selectWindowsPhysicalInterface(adapters, []windows.MibIpForwardRow2{
		route(28, 21), route(3, 1),
	}); got != "Ethernet" {
		t.Fatalf("selected %q, want Ethernet with lower interface index", got)
	}
	if got := selectWindowsPhysicalInterface(adapters, []windows.MibIpForwardRow2{
		virtual, nonDefault, onLink,
	}); got != "" {
		t.Fatalf("selected %q without a usable hardware default route", got)
	}
}

func TestWindowsForwardTableLayout(t *testing.T) {
	if size := unsafe.Sizeof(windows.MibIpForwardRow2{}); size != 104 {
		t.Fatalf("unexpected MIB_IPFORWARD_ROW2 size: %d", size)
	}
	if offset := unsafe.Offsetof(windows.MibIpForwardTable2{}.Table); offset != 8 {
		t.Fatalf("unexpected offset of MIB_IPFORWARD_TABLE2.Table: %d", offset)
	}
}

func BenchmarkWindowsPhysicalInterface(b *testing.B) {
	ctx := context.Background()
	name, err := windowsPhysicalInterface(ctx)
	if err != nil {
		b.Skip(err)
	}
	b.Logf("selected physical interface: %s", name)
	for b.Loop() {
		if got, err := windowsPhysicalInterface(ctx); err != nil || got != name {
			b.Fatalf("selected %q (%v), expected %q", got, err, name)
		}
	}
}
