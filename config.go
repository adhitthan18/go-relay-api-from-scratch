package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type config struct {
	Port           string
	DSN            string
	AdminEmail     string
	AdminPassword  string
	CookieSecure   bool
	BootstrapKey   string
	TrustedProxies []*net.IPNet
}

// parseTrustedProxies mengubah daftar dari environment menjadi jaringan.
// Alamat tunggal boleh ditulis apa adanya — ia dianggap /32 atau /128.
func parseTrustedProxies(daftar string) ([]*net.IPNet, error) {
	var hasil []*net.IPNet

	for _, bagian := range strings.Split(daftar, ",") {
		bagian = strings.TrimSpace(bagian)
		if bagian == "" {
			continue
		}

		if !strings.Contains(bagian, "/") {
			ip := net.ParseIP(bagian)
			if ip == nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q bukan alamat yang sah", bagian)
			}
			if ip.To4() != nil {
				bagian += "/32"
			} else {
				bagian += "/128"
			}
		}

		_, jaringan, err := net.ParseCIDR(bagian)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %q bukan CIDR yang sah", bagian)
		}
		hasil = append(hasil, jaringan)
	}

	return hasil, nil
}

// loadEnvFile mengisi environment dari berkas, hanya untuk kunci yang BELUM ada.
// Berkasnya boleh tidak ada — di server, systemd yang mengisi environment.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("membuka %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	nomor := 0

	for scanner.Scan() {
		nomor++
		baris := strings.TrimSpace(scanner.Text())

		if baris == "" || strings.HasPrefix(baris, "#") {
			continue
		}

		kunci, nilai, ketemu := strings.Cut(baris, "=")
		if !ketemu {
			return fmt.Errorf("%s baris %d: tidak ada tanda '='", path, nomor)
		}

		kunci = strings.TrimSpace(kunci)
		nilai = strings.Trim(strings.TrimSpace(nilai), `"'`)

		if _, sudahAda := os.LookupEnv(kunci); sudahAda {
			continue
		}
		if err := os.Setenv(kunci, nilai); err != nil {
			return fmt.Errorf("mengatur %s: %w", kunci, err)
		}
	}

	return scanner.Err()
}

func loadConfig() (*config, error) {
	if err := loadEnvFile(".env"); err != nil {
		return nil, err
	}

	host := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")
	bootstrapKey := os.Getenv("BOOTSTRAP_KEY")

	var kurang []string
	for kunci, nilai := range map[string]string{
		"DB_HOST":       host,
		"DB_PORT":       dbPort,
		"DB_USER":       user,
		"DB_PASSWORD":   password,
		"DB_NAME":       name,
		"BOOTSTRAP_KEY": bootstrapKey,
	} {
		if nilai == "" {
			kurang = append(kurang, kunci)
		}
	}
	if len(kurang) > 0 {
		// Urutan map acak; diurutkan supaya pesannya sama di setiap percobaan.
		slices.Sort(kurang)
		return nil, fmt.Errorf("environment wajib belum diisi: %s", strings.Join(kurang, ", "))
	}

	// Dibangun oleh driver-nya sendiri, bukan dirangkai dengan tanda tambah.
	// Password dengan '@', ':', atau '/' aman karena driver yang mengurus
	// penyandiannya.
	my := mysql.NewConfig()
	my.User = user
	my.Passwd = password
	my.Net = "tcp"
	my.Addr = host + ":" + dbPort
	my.DBName = name
	my.ParseTime = true
	my.Loc = nil
	my.AllowNativePasswords = true

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Default aman: true. Yang harus menyatakan "matikan" adalah lingkungan
	// pengembangan, bukan sebaliknya.
	cookieSecure := os.Getenv("COOKIE_SECURE") != "false"

	// Kosong berarti tidak mempercayai siapa pun — dan itu default yang benar.
	// Server yang salah pasang lebih baik menghitung semua orang sebagai satu
	// alamat daripada mempercayai header yang bisa dipalsukan.
	trusted, err := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return nil, err
	}

	return &config{
		Port:           port,
		DSN:            my.FormatDSN(),
		AdminEmail:     os.Getenv("ADMIN_EMAIL"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		CookieSecure:   cookieSecure,
		BootstrapKey:   bootstrapKey,
		TrustedProxies: trusted,
	}, nil
}
