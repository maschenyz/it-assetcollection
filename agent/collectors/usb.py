import platform
import wmi
import pythoncom

def get_usb_devices():
    """Scan for connected USB devices (Windows only)"""
    pythoncom.CoInitialize()
    usb_list = []
    if platform.system() == "Windows":
        try:
            c = wmi.WMI()
            for usb in c.Win32_USBHub():
                if usb.Name and "Root Hub" not in usb.Name:
                    usb_list.append({
                        "name": usb.Name,
                        "device_id": usb.DeviceID,
                        "status": usb.Status
                    })
        except:
            pass
    return usb_list
