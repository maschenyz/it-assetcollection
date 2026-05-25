document.addEventListener('alpine:init', () => {
    Alpine.data('dashboardApp', () => ({
        view: 'dashboard',
        viewTitle: '📊 Overview',
        connected: false,
        loading: false,
        stats: { total: 0, online: 0, offline: 0 },
        devices: [],
        software: [],
        screenshots: [],
        logs: [],
        printers: [],
        newPrinter: { name: '', brand: '', model: '', ip: '', type: 'Laser', department_id: null },
        showAddPrinter: false,
        selectedPrinter: null,
        printerLogs: [],
        showPrinterHistory: false,
        blacklist: [],
        settings: { TELEGRAM_BOT_TOKEN: '', TELEGRAM_CHAT_ID: '', LINE_NOTIFY_TOKEN: '', overtime_check_hour: '20', heartbeat_timeout_minutes: '10' },
        buildings: [],
        locations: [],
        newBuilding: { name: '', code: '' },
        activeTab: 'specs', // for device detail

        searchQuery: '',
        swSearchQuery: '',
        newBlacklist: { name: '', reason: '' },
        fullImage: null,
        detailDevice: null,
        showEditAssetModal: false,
        editAssetForm: {},
        userRole: localStorage.getItem('yuna_role') || 'viewer',

        // เพิ่มด้านใน Alpine.data('dashboardApp')
        dashboardPageData: {
            stats: { computers: 0, monitors: 0, printers: 0, software: 0, licenses: 0 },
            statusStats: [],
            manufacturerStats: [],
            typeStats: [],
            monitorBrands: [],

            get authHeaders() {
                return {
                    'Authorization': `Bearer ${localStorage.getItem('yuna_access_token')}`,
                    'Content-Type': 'application/json'
                };
            },

            async initDashboard() {
                try {
                    let res = await fetch('/api/v1/dashboard/metrics', { headers: this.authHeaders });

                    if (res.status === 401) {
                        localStorage.removeItem('yuna_access_token');
                        localStorage.removeItem('yuna_role');
                        window.location.href = '/login.html';
                        return;
                    }

                    // Fallback to legacy Python endpoint (limited data)
                    if (res.status === 404) {
                        res = await fetch('/api/v1/dashboard/summary', { headers: this.authHeaders });
                    }

                    if (!res.ok) throw new Error('โหลดไม่สำเร็จ');

                    const data = await res.json();
                    if (data?.kpis) {
                        const kpis = data.kpis || {};
                        this.stats = {
                            computers: kpis.computers || 0,
                            monitors: kpis.monitors || 0,
                            printers: kpis.printers || 0,
                            software: kpis.software || 0,
                            licenses: kpis.licenses || 0,
                        };

                        const statusMap = data.computers_by_status || {};
                        this.statusStats = Object.entries(statusMap).map(([name, count]) => ({ name, count }));

                        const mfg = Array.isArray(data.computers_by_manufacturer) ? data.computers_by_manufacturer : [];
                        this.manufacturerStats = mfg
                            .map(x => ({ name: x?.name ?? 'Unknown', count: x?.value ?? 0 }))
                            .sort((a, b) => b.count - a.count);

                        const types = Array.isArray(data.computers_by_type) ? data.computers_by_type : [];
                        this.typeStats = types
                            .map(x => ({ name: x?.name ?? 'Unknown', count: x?.value ?? 0 }))
                            .sort((a, b) => b.count - a.count);

                        const mons = Array.isArray(data.monitors_by_manufacturer) ? data.monitors_by_manufacturer : [];
                        this.monitorBrands = mons
                            .map(x => ({ name: x?.name ?? 'Unknown', count: x?.value ?? 0 }))
                            .sort((a, b) => b.count - a.count);
                        return;
                    }

                    // Legacy shape: { devices: { total, online }, software_total, alerts }
                    const devices = data?.devices || {};
                    this.stats = {
                        computers: devices.total || 0,
                        monitors: 0,
                        printers: 0,
                        software: data?.software_total || 0,
                        licenses: 0,
                    };
                    this.statusStats = [
                        { name: 'Ready (Online)', count: devices.online || 0 },
                        { name: 'Unavailable (Offline)', count: Math.max(0, (devices.total || 0) - (devices.online || 0)) },
                    ];
                    this.manufacturerStats = [];
                    this.typeStats = [];
                    this.monitorBrands = [];
                } catch (e) {
                    console.error(e);
                }
            }
        },

        async init() {
            this.checkAuth();
            await this.refreshData();
            // Load initial logs for dashboard
            this.loadLogs();

            const urlView = new URLSearchParams(window.location.search).get('view');
            if (urlView) {
                const allowed = new Set([
                    'dashboard',
                    'devices',
                    'software',
                    'printers',
                    'live',
                    'logs',
                    'screenshots',
                    'blacklist',
                    'master',
                    'reports',
                    'settings',
                ]);
                if (allowed.has(urlView)) this.switchView(urlView);
            }
            setInterval(() => this.refreshData(), 30000);
        },

        async exportCSV() {
            const res = await fetch('/api/v1/export/devices', { headers: this.authHeaders });
            if (!res.ok) return this.showToast('❌ Export failed');
            const blob = await res.blob();
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `yuna_report_${new Date().toISOString().slice(0, 10)}.csv`;
            document.body.appendChild(a);
            a.click();
            window.URL.revokeObjectURL(url);
        },

        checkAuth() {
            if (!localStorage.getItem('yuna_access_token')) {
                window.location.href = '/login.html';
            }
        },

        logout() {
            localStorage.removeItem('yuna_access_token');
            localStorage.removeItem('yuna_role');
            window.location.href = '/login.html';
        },

        get authHeaders() {
            return {
                'Authorization': `Bearer ${localStorage.getItem('yuna_access_token')}`,
                'Content-Type': 'application/json'
            };
        },

        async refreshData() {
            try {
                this.loading = true;
                const res = await fetch('/api/v1/devices', { headers: this.authHeaders });
                if (res.status === 401) return this.logout();
                this.devices = await res.json();
                this.stats.total = this.devices.length;
                this.stats.online = this.devices.filter(d => d.status === 'ONLINE').length;
                this.stats.offline = this.devices.filter(d => d.status === 'OFFLINE').length;
                this.connected = true;
            } catch (err) {
                this.connected = false;
            } finally {
                this.loading = false;
            }
        },

        async loadMasterData() {
            try {
                const res1 = await fetch('/api/v1/master/buildings', { headers: this.authHeaders });
                this.buildings = await res1.json();
                const res2 = await fetch('/api/v1/master/locations', { headers: this.authHeaders });
                this.locations = await res2.json();
            } catch (err) {
                console.error("Error loading master data:", err);
            }
        },

        async addBuilding() {
            if (!this.newBuilding.name) return;
            const res = await fetch('/api/v1/master/buildings', {
                method: 'POST',
                headers: this.authHeaders,
                body: JSON.stringify(this.newBuilding)
            });
            if (res.ok) {
                this.showToast('🏢 บันทึกข้อมูลตึกแล้วค่ะ');
                this.newBuilding = { name: '', code: '' };
                this.loadMasterData();
            }
        },

        async switchView(v) {
            this.view = v;
            this.detailDevice = null;
            const titles = {
                dashboard: '📊 Overview',
                devices: '💻 Computer Inventory',
                software: '📦 Software Registry',
                printers: '🖨️ Printer Registry',
                live: '⚡ Live Control Center',
                logs: '📜 System Audit Logs',
                screenshots: '📸 Recent Screenshots',
                blacklist: '🚫 Software Blacklist',
                master: '🏢 Master Data',
                reports: '📊 System Reports',
                settings: '⚙️ System Settings',
                detail: '📋 Device Detail'
            };
            this.viewTitle = titles[v] || 'Yuna Asset';

            if (v === 'software') this.loadSoftware();
            if (v === 'logs') this.loadLogs();
            if (v === 'printers') this.loadPrinters();
            if (v === 'screenshots') this.loadScreenshots();
            if (v === 'blacklist') this.loadBlacklist();
            if (v === 'settings') this.loadSettings();
            if (v === 'master') this.loadMasterData();
        },

        // --- Data Fetchers ---
        async loadSoftware() {
            const res = await fetch('/api/v1/software', { headers: this.authHeaders });
            this.software = await res.json();
        },
        async loadLogs() {
            const res = await fetch('/api/v1/logs', { headers: this.authHeaders });
            this.logs = await res.json();
        },
        async loadPrinters() {
            const res = await fetch('/api/v1/printers', { headers: this.authHeaders });
            this.printers = await res.json();
        },
        async loadBlacklist() {
            const res = await fetch('/api/v1/blacklist', { headers: this.authHeaders });
            this.blacklist = await res.json();
        },
        async loadSettings() {
            const res = await fetch('/api/v1/settings', { headers: this.authHeaders });
            this.settings = await res.json();
        },
        async loadScreenshots() {
            const res = await fetch('/api/v1/screenshots', { headers: this.authHeaders });
            this.screenshots = await res.json();
        },

        // --- Actions ---
        async saveSettings() {
            const res = await fetch('/api/v1/settings', {
                method: 'POST',
                headers: this.authHeaders,
                body: JSON.stringify(this.settings)
            });
            if (res.ok) this.showToast('✅ บันทึกการตั้งค่าแล้วค่ะ');
        },

        async addBlacklist() {
            if (!this.newBlacklist.name) return;
            const res = await fetch('/api/v1/blacklist', {
                method: 'POST',
                headers: this.authHeaders,
                body: JSON.stringify(this.newBlacklist)
            });
            if (res.ok) {
                this.showToast('🚫 เพิ่มโปรแกรมต้องห้ามแล้วค่ะ');
                this.newBlacklist = { name: '', reason: '' };
                this.loadBlacklist();
            }
        },

        async deleteBlacklist(id) {
            if (!confirm('ยืนยันการลบกฎนี้ไหมคะ?')) return;
            const res = await fetch(`/api/v1/blacklist/${id}`, { method: 'DELETE', headers: this.authHeaders });
            if (res.ok) {
                this.showToast('🗑️ ลบกฎแล้วค่ะ');
                this.loadBlacklist();
            }
        },

        async openPrinterHistory(p) {
            this.selectedPrinter = p;
            this.printerLogs = [];
            this.showPrinterHistory = true;
            const res = await fetch(`/api/v1/printers/${p.id}/logs`, { headers: this.authHeaders });
            if (res.ok) this.printerLogs = await res.json();
        },

        async addPrinter() {
            if (!this.newPrinter.name) return;
            const res = await fetch('/api/v1/printers', {
                method: 'POST',
                headers: this.authHeaders,
                body: JSON.stringify(this.newPrinter)
            });
            if (res.ok) {
                this.showToast('🖨️ บันทึกข้อมูลเครื่องพิมพ์แล้วค่ะ');
                this.newPrinter = { name: '', brand: '', model: '', ip: '', type: 'Laser', department_id: null };
                this.showAddPrinter = false;
                this.loadPrinters();
            }
        },

        async updateInk(id) {
            const res = await fetch(`/api/v1/printers/${id}/ink`, { method: 'POST', headers: this.authHeaders });
            if (res.ok) {
                this.showToast('💧 บันทึกการเปลี่ยนหมึกแล้วค่ะ');
                this.loadPrinters();
            }
        },

        async sendCommand(uuid, cmd, label) {
            if (!confirm(`ส่งคำสั่ง "${label}" ใช่ไหมคะ?`)) return;
            const res = await fetch('/api/v1/tasks', {
                method: 'POST',
                headers: this.authHeaders,
                body: JSON.stringify({ device_uuid: uuid, command_type: cmd, payload: {} })
            });
            if (res.ok) this.showToast(`✅ ส่งคำสั่ง "${label}" แล้วค่ะ`);
        },

        async openDetail(dev) {
            this.activeTab = 'specs';
            this.detailDevice = { ...dev, _loading: true };
            this.switchView('detail');
            await this.loadMasterData();
            const res = await fetch(`/api/v1/devices/${dev.uuid}`, { headers: this.authHeaders });
            this.detailDevice = { ...await res.json(), _loading: false };
        },

        async updateDeviceMetadata() {
            if (!this.detailDevice) return;
            try {
                this.loading = true;
                const payload = {
                    asset_number: this.detailDevice.asset_number || '',
                    location_id: this.detailDevice.location_id ? parseInt(this.detailDevice.location_id) : null
                };
                const res = await fetch(`/api/v1/devices/${this.detailDevice.uuid}`, {
                    method: 'PUT',
                    headers: this.authHeaders,
                    body: JSON.stringify(payload)
                });
                if (res.ok) {
                    this.showToast('💾 บันทึกข้อมูลสินทรัพย์เรียบร้อยแล้วค่ะ แบงค์!');
                    const updatedRes = await fetch(`/api/v1/devices/${this.detailDevice.uuid}`, { headers: this.authHeaders });
                    this.detailDevice = { ...await updatedRes.json(), _loading: false };
                    await this.refreshData();
                } else {
                    const text = await res.text();
                    this.showToast(`❌ บันทึกไม่สำเร็จ: ${text}`);
                }
            } catch (err) {
                this.showToast(`❌ เกิดข้อผิดพลาด: ${err.message}`);
            } finally {
                this.loading = false;
            }
        },

        openEditAssetModal() {
            if (!this.detailDevice) return;
            const md = this.detailDevice.master_data || {};
            this.editAssetForm = {
                sku: md.sku || '',
                type_ict: md.type_ict || '',
                purchase_date: md.purchase_date || '',
                warranty_expire: md.warranty_expire || '',
                warranty_months: md.warranty_months || 0,
                amount: md.amount || 0,
                comment: md.comment || ''
            };
            this.showEditAssetModal = true;
        },

        async saveAssetInfo() {
            if (!this.detailDevice) return;
            try {
                this.loading = true;
                const payload = {
                    sku: this.editAssetForm.sku,
                    type_ict: this.editAssetForm.type_ict,
                    purchase_date: this.editAssetForm.purchase_date,
                    warranty_expire: this.editAssetForm.warranty_expire,
                    warranty_months: parseInt(this.editAssetForm.warranty_months) || 0,
                    amount: parseFloat(this.editAssetForm.amount) || 0,
                    comment: this.editAssetForm.comment
                };
                
                const res = await fetch(`/api/v1/devices/${this.detailDevice.uuid}/asset`, {
                    method: 'PUT',
                    headers: this.authHeaders,
                    body: JSON.stringify(payload)
                });
                
                if (res.ok) {
                    this.showToast('💾 อัปเดตข้อมูลคุมทรัพย์สินเรียบร้อยแล้วค่ะ!');
                    this.showEditAssetModal = false;
                    const updatedRes = await fetch(`/api/v1/devices/${this.detailDevice.uuid}`, { headers: this.authHeaders });
                    this.detailDevice = { ...await updatedRes.json(), _loading: false };
                } else {
                    const text = await res.text();
                    this.showToast(`❌ บันทึกไม่สำเร็จ: ${text}`);
                }
            } catch (err) {
                this.showToast(`❌ เกิดข้อผิดพลาด: ${err.message}`);
            } finally {
                this.loading = false;
            }
        },

        async deleteDevice(uuid) {
            if (!confirm('คุณแบงค์แน่ใจไหมคะว่าจะลบเครื่องคอมพิวเตอร์นี้ออกจากระบบ? ข้อมูลประวัติทั้งหมดจะถูกลบออกถาวรเลยค่ะ 🗑️')) return;
            try {
                this.loading = true;
                const res = await fetch(`/api/v1/devices/${uuid}`, {
                    method: 'DELETE',
                    headers: this.authHeaders
                });
                if (res.ok) {
                    this.showToast('🗑️ ลบเครื่องคอมพิวเตอร์เรียบร้อยแล้วค่ะ');
                    await this.refreshData();
                    this.switchView('devices');
                } else {
                    const text = await res.text();
                    this.showToast(`❌ ลบไม่สำเร็จ: ${text}`);
                }
            } catch (err) {
                this.showToast(`❌ ลบไม่สำเร็จ: ${err.message}`);
            } finally {
                this.loading = false;
            }
        },

        async viewFullImage(id) {
            const res = await fetch(`/api/v1/screenshots/${id}`, { headers: this.authHeaders });
            const data = await res.json();
            this.fullImage = data.image.startsWith('data:') ? data.image : 'data:image/jpeg;base64,' + data.image;
        },

        // --- Helpers ---
        get filteredDevices() {
            const q = this.searchQuery.toLowerCase();
            return this.devices.filter(d =>
                d.hostname.toLowerCase().includes(q) ||
                d.ip_address.includes(q) ||
                (d.last_user || '').toLowerCase().includes(q)
            );
        },
        formatDate(str) {
            if (!str) return '-';
            return new Date(str).toLocaleString('th-TH', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
        },
        getOsLabel(dev) { return dev.os_info?.caption || dev.os_info?.os || (dev.windows_build ? `Windows (Build ${dev.windows_build})` : (dev.os_info?.system || 'N/A')); },
        showToast(msg) {
            const t = document.createElement('div'); t.className = 'toast-msg show'; t.textContent = msg;
            document.body.appendChild(t); setTimeout(() => { t.classList.remove('show'); setTimeout(() => t.remove(), 400); }, 3000);
        }
    }));
});
