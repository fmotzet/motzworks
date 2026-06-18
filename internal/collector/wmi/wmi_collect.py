#!/usr/bin/env python3
"""motzworks agentless Windows inventory via WMI over DCOM (impacket).

This is the transport the discontinued Spiceworks Inventory used: WMI/DCOM with
NTLM, which works against hosts whose WinRM listener is Kerberos-only.

Connection parameters are read from the environment (never argv, so the password
never appears in the process list). A single JSON inventory document is written
to stdout; diagnostics go to stderr. Exit codes: 0 ok, 2 auth/connection error,
3 usage error.
"""
import json
import os
import sys

try:
    from impacket.dcerpc.v5.dcomrt import DCOMConnection
    from impacket.dcerpc.v5.dcom import wmi
    from impacket.dcerpc.v5.dtypes import NULL
except Exception as e:  # impacket not installed
    sys.stderr.write("impacket import failed: %s\n" % e)
    sys.exit(3)

HKLM = 0x80000002
UNINSTALL_PATHS = [
    "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall",
    "SOFTWARE\\Wow6432Node\\Microsoft\\Windows\\CurrentVersion\\Uninstall",
]


def props(obj):
    """Flatten an impacket WMI object's properties to {name: value}."""
    out = {}
    for name, meta in obj.getProperties().items():
        out[name] = meta["value"]
    return out


def query_all(svc, wql):
    rows = []
    it = svc.ExecQuery(wql)
    while True:
        try:
            obj = it.Next(0xFFFFFFFF, 1)[0]
        except Exception as e:
            if "S_FALSE" in str(e):
                break
            raise
        rows.append(props(obj))
    return rows


def query_one(svc, wql):
    rows = query_all(svc, wql)
    return rows[0] if rows else {}


def collect_software(svc):
    """Installed software from the registry Uninstall keys via StdRegProv."""
    apps = []
    try:
        reg, _ = svc.GetObject("StdRegProv")
    except Exception as e:
        sys.stderr.write("StdRegProv unavailable: %s\n" % e)
        return apps
    for base in UNINSTALL_PATHS:
        try:
            names = reg.EnumKey(HKLM, base).sNames or []
        except Exception:
            continue
        for sub in names:
            path = base + "\\" + sub
            try:
                name = reg.GetStringValue(HKLM, path, "DisplayName").sValue
            except Exception:
                name = None
            if not name:
                continue

            def sval(value):
                try:
                    return reg.GetStringValue(HKLM, path, value).sValue or ""
                except Exception:
                    return ""

            apps.append({
                "name": name,
                "version": sval("DisplayVersion"),
                "publisher": sval("Publisher"),
            })
    return apps


def main():
    try:
        addr = os.environ["WMI_ADDR"]
        user = os.environ["WMI_USER"]
    except KeyError as e:
        sys.stderr.write("missing env %s\n" % e)
        return 3
    password = os.environ.get("WMI_PASS", "")
    domain = os.environ.get("WMI_DOMAIN", "")

    try:
        dcom = DCOMConnection(addr, user, password, domain, "", "", "", oxidResolver=True, doKerberos=False)
    except Exception as e:
        sys.stderr.write("connect/auth failed: %s\n" % e)
        return 2

    try:
        iface = dcom.CoCreateInstanceEx(wmi.CLSID_WbemLevel1Login, wmi.IID_IWbemLevel1Login)
        login = wmi.IWbemLevel1Login(iface)
        svc = login.NTLMLogin("//./root/cimv2", NULL, NULL)
        login.RemRelease()

        out = {
            "os": query_one(svc, "SELECT Caption,Version,BuildNumber,OSArchitecture,CSName FROM Win32_OperatingSystem"),
            "cs": query_one(svc, "SELECT Name,Manufacturer,Model,TotalPhysicalMemory FROM Win32_ComputerSystem"),
            "bios": query_one(svc, "SELECT SerialNumber FROM Win32_BIOS"),
            "cpu": query_one(svc, "SELECT Name,NumberOfLogicalProcessors FROM Win32_Processor"),
            "net": query_all(svc, "SELECT Description,MACAddress,IPAddress FROM Win32_NetworkAdapterConfiguration WHERE IPEnabled=TRUE"),
            "users": query_all(svc, "SELECT Name,FullName FROM Win32_UserAccount WHERE LocalAccount=TRUE"),
            "software": collect_software(svc),
        }
        json.dump(out, sys.stdout, default=str)
        return 0
    except Exception as e:
        sys.stderr.write("wmi query failed: %s\n" % e)
        return 2
    finally:
        try:
            dcom.disconnect()
        except Exception:
            pass


if __name__ == "__main__":
    sys.exit(main())
