package managedproxy

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsHardwareInterface = 1 << 0 // MIB_IF_ROW2.InterfaceAndOperStatusFlags.HardwareInterface

var (
	errNoWindowsInterface    = errors.New("未找到可用的物理 IPv4 网卡。请确认以太网或 Wi-Fi 已连接并取得 IPv4 地址。CCBox 外网保持关闭")
	errNoWindowsDefaultRoute = errors.New("Windows 物理网卡没有可用的 IPv4 默认路由。请检查以太网或 Wi-Fi 是否已取得默认网关，再继续安装或重试检测。CCBox 外网保持关闭")
)

func windowsPhysicalInterface(ctx context.Context) (string, error) {
	// IP Helper calls are synchronous. Check the deadline between calls and
	// reject late results when a call returns.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	adapters, err := windowsPhysicalAdapters(ctx)
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("Windows 物理网卡查询超时；CCBox 外网保持关闭: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("无法读取 Windows 物理网卡信息；CCBox 外网保持关闭: %w", err)
	}
	if len(adapters) == 0 {
		return "", errNoWindowsInterface
	}
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return "", fmt.Errorf("无法读取 Windows IPv4 路由；CCBox 外网保持关闭: %w", err)
	}
	if table == nil {
		return "", errNoWindowsDefaultRoute
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("Windows IPv4 路由查询超时；CCBox 外网保持关闭: %w", err)
	}
	name := selectWindowsPhysicalInterface(adapters, table.Rows())
	if name == "" {
		return "", errNoWindowsDefaultRoute
	}
	return name, nil
}

func windowsPhysicalAdapters(ctx context.Context) (map[uint32]windowsPhysicalAdapter, error) {
	size := uint32(15000) // Recommended initial buffer for GetAdaptersAddresses.
	flags := uint32(windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER |
		windows.GAA_FLAG_INCLUDE_ALL_INTERFACES)
	var buffer []byte
	var first *windows.IpAdapterAddresses
	for attempts := 0; ; attempts++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		buffer = make([]byte, size)
		first = (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
		// AF_UNSPEC also lists hardware adapters whose IPv4 binding moved to a
		// Hyper-V external switch; IPv4 eligibility is checked per adapter.
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) && size > uint32(len(buffer)) && attempts < 3 {
			continue
		}
		if errors.Is(err, windows.ERROR_NO_DATA) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		break
	}

	var list []windowsAdapterInfo
	var lookupErr error
	for adapter := first; adapter != nil; adapter = adapter.Next {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := readWindowsAdapterInfo(adapter, func(row *windows.MibIfRow2) error {
			return windows.GetIfEntry2Ex(windows.MibIfEntryNormalWithoutStatistics, row)
		})
		if err != nil {
			if info.ipv4 && lookupErr == nil {
				lookupErr = err
			}
			continue
		}
		if info.index != 0 {
			list = append(list, info)
		}
	}
	runtime.KeepAlive(buffer)
	uplinks := windowsUplinkAdapters(list)
	if len(uplinks) == 0 && lookupErr != nil {
		return nil, lookupErr
	}
	return uplinks, nil
}

func readWindowsAdapterInfo(adapter *windows.IpAdapterAddresses, lookup func(*windows.MibIfRow2) error) (windowsAdapterInfo, error) {
	if adapter.OperStatus != windows.IfOperStatusUp || (adapter.IfIndex == 0 && adapter.Luid == 0) {
		return windowsAdapterInfo{}, nil
	}
	// IfIndex may be zero when TCP/IPv4 belongs to a Hyper-V management
	// adapter. The LUID still identifies the underlying physical NIC.
	row := windows.MibIfRow2{InterfaceLuid: adapter.Luid, InterfaceIndex: adapter.IfIndex}
	ipv4 := adapter.IfIndex != 0 && hasPreferredWindowsIPv4(adapter.FirstUnicastAddress)
	if err := lookup(&row); err != nil {
		return windowsAdapterInfo{ipv4: ipv4}, err
	}
	if row.OperStatus != windows.IfOperStatusUp || (adapter.IfIndex != 0 && row.InterfaceIndex != adapter.IfIndex) {
		return windowsAdapterInfo{}, nil
	}
	info := windowsAdapterInfo{
		index:    row.InterfaceIndex,
		metric:   adapter.Ipv4Metric,
		hardware: row.InterfaceAndOperStatusFlags&windowsHardwareInterface != 0,
		ipv4:     ipv4,
	}
	if n := min(int(adapter.PhysicalAddressLength), len(adapter.PhysicalAddress)); n > 0 {
		info.mac = append([]byte(nil), adapter.PhysicalAddress[:n]...)
	} else if n := min(int(row.PhysicalAddressLength), len(row.PhysicalAddress)); n > 0 {
		info.mac = append([]byte(nil), row.PhysicalAddress[:n]...)
	}
	if adapter.FriendlyName != nil {
		info.name = windows.UTF16PtrToString(adapter.FriendlyName)
	}
	return info, nil
}

func hasPreferredWindowsIPv4(first *windows.IpAdapterUnicastAddress) bool {
	for address := first; address != nil; address = address.Next {
		if address.DadState != windows.IpDadStatePreferred || address.Address.Sockaddr == nil {
			continue
		}
		ip := address.Address.IP()
		if ip.To4() != nil && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() && !ip.IsUnspecified() {
			return true
		}
	}
	return false
}

func selectWindowsPhysicalInterface(adapters map[uint32]windowsPhysicalAdapter, routes []windows.MibIpForwardRow2) string {
	var bestName string
	var bestMetric uint64
	var bestIndex uint32
	for _, route := range routes {
		adapter, ok := adapters[route.InterfaceIndex]
		if !ok || route.DestinationPrefix.Prefix.Family != windows.AF_INET || route.DestinationPrefix.PrefixLength != 0 ||
			route.NextHop.Family != windows.AF_INET || route.Loopback != 0 {
			continue
		}
		nextHop := (*windows.RawSockaddrInet4)(unsafe.Pointer(&route.NextHop))
		if nextHop.Addr == [4]byte{} {
			continue
		}
		metric := uint64(route.Metric) + uint64(adapter.metric)
		if bestName == "" || metric < bestMetric || (metric == bestMetric && route.InterfaceIndex < bestIndex) {
			bestName, bestMetric, bestIndex = adapter.name, metric, route.InterfaceIndex
		}
	}
	return bestName
}
