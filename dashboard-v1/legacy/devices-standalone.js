function devicesPage() {
    return {
        devices: [],
        searchQuery: '',

        async init() {
            await this.loadDevices()
        },

        async loadDevices() {
            try {
                const res = await fetch('/api/v1/devices')
                this.devices = await res.json()
            } catch (err) {
                console.error(err)
            }
        },

        get filteredDevices() {
            return this.devices.filter(d => {
                return (
                    d.hostname?.toLowerCase().includes(this.searchQuery.toLowerCase()) ||
                    d.ip_address?.toLowerCase().includes(this.searchQuery.toLowerCase())
                )
            })
        },

        getOsLabel(dev) {
            return dev.os_info?.caption || 'Unknown OS'
        }
    }
}
