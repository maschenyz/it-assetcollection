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
        monitors: [],
        newPrinter: { name: '', brand: '', model: '', ip: '', type: 'Laser', department_id: null },
        showAddPrinter: false,
        selectedPrinter: null,
        printerLogs: [],
        showPrinterHistory: false,
        blacklist: [],
        settings: { TELEGRAM_BOT_TOKEN: '', TELEGRAM_CHAT_ID: '', LINE_NOTIFY_TOKEN: '', overtime_check_hour: '20', heartbeat_timeout_minutes: '10' },
        buildings: [],
        newBuilding: { name: '', code: '' },
        activeTab: 'specs', // for device detail
        
        searchQuery: '',
        swSearchQuery: '',
        newBlacklist: { name: '', reason: '' },
        fullImage: null,
        detailDevice: null,
        userRole: localStorage.getItem('yuna_role') || 'viewer',

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
                    'monitors',
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
            a.download = `yuna_report_${new Date().toISOString().slice(0,10)}.csv`;
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
                this.stats.total   = this.devices.length;
                this.stats.online  = this.devices.filter(d => d.status === 'ONLINE').length;
                this.stats.offline = this.devices.filter(d => d.status === 'OFFLINE').length;
                this.connected = true;
            } catch (err) {
                this.connected = false;
            } finally {
                this.loading = false;
            }
        },

        async loadMasterData() {
            const res = await fetch('/api/v1/master/buildings', { headers: this.authHeaders });
            this.buildings = await res.json();
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
                devices:   '💻 Computer Inventory',
                software:  '📦 Software Registry',
                printers:  '🖨️ Printer Registry',
                monitors:  '🖥️ Monitor Inventory',
                live:      '⚡ Live Control Center',
                logs:      '📜 System Audit Logs',
                screenshots: '📸 Recent Screenshots',
                blacklist: '🚫 Software Blacklist',
                master:    '🏢 Master Data',
                reports:   '📊 System Reports',
                settings:  '⚙️ System Settings',
                detail:    '📋 Device Detail'
            };
            this.viewTitle = titles[v] || 'Yuna Asset';

            if (v === 'software') this.loadSoftware();
            if (v === 'logs') this.loadLogs();
            if (v === 'printers') this.loadPrinters();
            if (v === 'monitors') this.loadMonitors();
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
        async loadMonitors() {
            // Aggregate monitors from all devices
            const res = await fetch('/api/v1/devices', { headers: this.authHeaders });
            if (!res.ok) return;
            const devices = await res.json();
            const all = [];
            await Promise.all(devices.map(async (dev) => {
                try {
                    const r = await fetch(`/api/v1/devices/${dev.uuid}/monitors`, { headers: this.authHeaders });
                    if (!r.ok) return;
                    const mons = await r.json();
                    mons.forEach(m => all.push({ ...m, hostname: dev.hostname, ip: dev.ip_address }));
                } catch (_) {}
            }));
            this.monitors = all;
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
            const res = await fetch(`/api/v1/devices/${dev.uuid}`, { headers: this.authHeaders });
            this.detailDevice = { ...await res.json(), _loading: false };
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
        getOsLabel(dev) { return dev.windows_build ? `Windows (Build ${dev.windows_build})` : (dev.os_info?.system || 'N/A'); },
        showToast(msg) {
            const t = document.createElement('div'); t.className = 'toast-msg show'; t.textContent = msg;
            document.body.appendChild(t); setTimeout(() => { t.classList.remove('show'); setTimeout(() => t.remove(), 400); }, 3000);
        }
    }));
});
