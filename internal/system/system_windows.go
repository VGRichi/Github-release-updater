package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsSystem struct{}

// New returns the System for this OS.
func New() System { return windowsSystem{} }

var uninstallKeys = []struct {
	root    registry.Key
	rootStr string
	path    string
	perUser bool
}{
	{registry.CURRENT_USER, `HKCU`, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, true},
	{registry.LOCAL_MACHINE, `HKLM`, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, false},
	{registry.LOCAL_MACHINE, `HKLM`, `Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, false},
}

// Programs reads Installed apps from the three uninstall keys, skipping hidden system components.
func (windowsSystem) Programs() ([]Program, error) {
	var programs []Program
	for _, u := range uninstallKeys {
		parent, err := registry.OpenKey(u.root, u.path, registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
		if err != nil {
			continue
		}
		names, _ := parent.ReadSubKeyNames(-1)
		parent.Close()
		for _, name := range names {
			k, err := registry.OpenKey(u.root, u.path+`\`+name, registry.QUERY_VALUE|registry.WOW64_64KEY)
			if err != nil {
				continue
			}
			p := Program{Key: u.rootStr + `\` + u.path + `\` + name, PerUser: u.perUser}
			p.Name, _, _ = k.GetStringValue("DisplayName")
			p.Version, _, _ = k.GetStringValue("DisplayVersion")
			hidden, _, _ := k.GetIntegerValue("SystemComponent")
			msi, _, _ := k.GetIntegerValue("WindowsInstaller")
			p.MSI = msi == 1
			for _, v := range []string{"URLInfoAbout", "URLUpdateInfo", "HelpLink"} {
				if s, _, err := k.GetStringValue(v); err == nil && s != "" {
					p.Links = append(p.Links, s)
				}
			}
			k.Close()
			if p.Name != "" && hidden == 0 {
				programs = append(programs, p)
			}
		}
	}
	return programs, nil
}

type shellExecuteInfo struct {
	size          uint32
	mask          uint32
	hwnd          windows.Handle
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instApp       windows.Handle
	idList        uintptr
	class         *uint16
	keyClass      windows.Handle
	hotKey        uint32
	iconOrMonitor windows.Handle
	process       windows.Handle
}

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// RunInstaller starts the installer the way Explorer would, so one that needs admin rights shows the
// admin prompt, and waits for that one process: an installer that opens the app when it's done
// doesn't make Reforge wait until the app is closed.
func (windowsSystem) RunInstaller(file, args string) (int, error) {
	const seeMaskNoCloseProcess, seeMaskNoAsync, swShowNormal = 0x40, 0x100, 1
	info := shellExecuteInfo{mask: seeMaskNoCloseProcess | seeMaskNoAsync, show: swShowNormal}
	info.size = uint32(unsafe.Sizeof(info))
	if !filepath.IsAbs(file) { // like msiexec.exe: Windows' own, never one in the current folder
		if dir, err := windows.GetSystemDirectory(); err == nil {
			file = filepath.Join(dir, file)
		}
	}
	info.file, _ = windows.UTF16PtrFromString(file)
	if args != "" {
		info.parameters, _ = windows.UTF16PtrFromString(args)
	}
	if ok, _, err := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, errors.New("the admin prompt was declined")
		}
		return 0, fmt.Errorf("couldn't start the installer: %w", err)
	}
	if info.process == 0 {
		return 0, nil
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return 0, err
	}
	return int(code), nil
}

// Signer checks the file's Authenticode signature (what Windows shows as "Digital Signatures").
// Revocation isn't checked online, so a check never waits on the network.
func (windowsSystem) Signer(file string) (string, error) {
	path, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return "", err
	}
	data := &windows.WinTrustData{
		Size:             uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:         windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_NONE,
		UnionChoice:      windows.WTD_CHOICE_FILE,
		StateAction:      windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&windows.WinTrustFileInfo{
			Size:     uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})),
			FilePath: path,
		}),
	}
	verifyErr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	switch verifyErr {
	case nil:
		return signerName(path)
	case windows.Errno(windows.TRUST_E_NOSIGNATURE), windows.Errno(windows.TRUST_E_SUBJECT_FORM_UNKNOWN), windows.Errno(windows.TRUST_E_PROVIDER_UNKNOWN):
		return "", nil
	}
	return "", fmt.Errorf("its digital signature isn't valid (%v)", verifyErr)
}

var (
	crypt32          = windows.NewLazySystemDLL("crypt32.dll")
	cryptMsgGetParam = crypt32.NewProc("CryptMsgGetParam")
	cryptMsgClose    = crypt32.NewProc("CryptMsgClose")
)

// signerName reads who signed the file: the signer's certificate, found by its issuer and serial number.
func signerName(path *uint16) (string, error) {
	var encoding, contentType, formatType uint32
	var store, msg windows.Handle
	if err := windows.CryptQueryObject(windows.CERT_QUERY_OBJECT_FILE, unsafe.Pointer(path),
		windows.CERT_QUERY_CONTENT_FLAG_PKCS7_SIGNED_EMBED, windows.CERT_QUERY_FORMAT_FLAG_BINARY, 0,
		&encoding, &contentType, &formatType, &store, &msg, nil); err != nil {
		return "", err
	}
	defer windows.CertCloseStore(store, 0)
	defer cryptMsgClose.Call(uintptr(msg))

	const cmsgSignerInfoParam = 6
	var size uint32
	if ok, _, err := cryptMsgGetParam.Call(uintptr(msg), cmsgSignerInfoParam, 0, 0, uintptr(unsafe.Pointer(&size))); ok == 0 {
		return "", err
	}
	buf := make([]byte, size)
	if ok, _, err := cryptMsgGetParam.Call(uintptr(msg), cmsgSignerInfoParam, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); ok == 0 {
		return "", err
	}
	// The start of CMSG_SIGNER_INFO
	signer := (*struct {
		Version      uint32
		Issuer       windows.CertNameBlob
		SerialNumber windows.CryptIntegerBlob
	})(unsafe.Pointer(&buf[0]))
	info := windows.CertInfo{Issuer: signer.Issuer, SerialNumber: signer.SerialNumber}
	cert, err := windows.CertFindCertificateInStore(store, windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, 0,
		windows.CERT_FIND_SUBJECT_CERT, unsafe.Pointer(&info), nil)
	if err != nil {
		return "", err
	}
	defer windows.CertFreeCertificateContext(cert)
	n := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, nil, 0)
	name := make([]uint16, n)
	windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &name[0], n)
	return windows.UTF16ToString(name), nil
}

func (windowsSystem) IsAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }

// DownloadsFolder asks Windows where Downloads is, since it can be moved.
func DownloadsFolder() string {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if dir, _, err := k.GetStringValue("{374DE290-123F-4565-9164-39C4925E467B}"); err == nil && dir != "" {
			return dir
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

// DownloadOrigin reads the file's Zone.Identifier stream (the "Mark of the Web").
func (windowsSystem) DownloadOrigin(file string) string {
	data, err := os.ReadFile(file + ":Zone.Identifier")
	if err != nil {
		return ""
	}
	return string(data)
}

func (windowsSystem) WingetPackages() []Package {
	out, err := exec.Command("winget", "list", "--source", "winget", "--disable-interactivity", "--accept-source-agreements").Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	return parseWingetList(string(out))
}
