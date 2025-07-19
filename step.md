share-it
•	Tagline: Utilitas CLI fundamental untuk berbagi file rahasia secara aman.
Deskripsi Detail & Visi
Di alur kerja developer, seringkali ada kebutuhan untuk berbagi data sensitif secara cepat—kunci API, file konfigurasi, atau log—tanpa harus mengunggahnya ke platform penyimpanan permanen seperti Google Drive atau Dropbox. share-it adalah solusi untuk masalah ini.
Ia adalah sebuah utilitas Command-Line Interface (CLI) yang sangat ringan, dibangun dengan satu prinsip: satu perintah, satu URL unik, satu kali unduh. Pengguna cukup menjalankan share-it [nama_file] di terminal. Alat ini akan secara otomatis menjalankan web server sementara, menghasilkan URL unik yang dilindungi token, dan setelah file tersebut diunduh satu kali, server dan file tersebut akan musnah secara otomatis. Tidak ada jejak, tidak ada risiko terekspos. Ini adalah alat wajib di toolkit setiap developer dan sysadmin.
Tumpukan Teknologi (Amunisi Detail)
•	Core Logic:
o	Bahasa: Go. Pilihan sempurna karena kemampuannya untuk dikompilasi menjadi satu file biner statis tanpa dependensi, membuatnya sangat portabel dan mudah didistribusikan. Konkurensi bawaan Go dengan goroutine juga sangat efisien untuk menangani permintaan web.
•	Web Server:
o	Menggunakan library standar Go: net/http. Sangat powerful dan lebih dari cukup untuk membuat web server sementara yang ringan tanpa perlu framework eksternal.
•	URL Generation:
o	Menggunakan library standar Go: crypto/rand untuk menghasilkan token acak yang aman secara kriptografis, memastikan URL tidak bisa ditebak.
•	Platform & DevOps:
o	Distribusi: Menggunakan GitHub Releases untuk mendistribusikan biner yang sudah dikompilasi untuk berbagai sistem operasi (Linux, macOS, Windows).
o	Deployment (untuk versi web opsional): Fly.io (dari Student Pack) sangat ideal karena bisa menjalankan biner Go secara langsung.
o	CI/CD: CircleCI (dari Student Pack) untuk secara otomatis meng-kompilasi dan merilis versi baru setiap kali ada commit ke main branch.
Tantangan Terberat (Detail Operasional)
1.	Manajemen State Sekali Pakai yang Aman: Bagaimana cara memastikan sebuah tautan benar-benar hanya bisa digunakan sekali, terutama jika ada dua orang yang mencoba mengunduhnya pada saat yang bersamaan (race condition)? Solusinya adalah menggunakan mekanisme locking yang aman di memori, seperti sync.Mutex atau channel sebagai semaphore di Go, untuk memastikan hanya satu permintaan unduhan yang bisa diproses.
2.	Penanganan File Besar Secara Efisien: Jika pengguna mencoba berbagi file berukuran 1GB, aplikasi tidak boleh memuat seluruh file ke dalam RAM karena akan menyebabkan crash. Tantangannya adalah mengimplementasikan streaming I/O, di mana file dibaca dari disk dan ditulis ke koneksi HTTP dalam potongan-potongan kecil (chunks), menjaga penggunaan memori tetap rendah dan konstan.
3.	Keamanan Protokol Transfer: Meskipun sederhana, protokolnya harus aman dari serangan dasar. Ini berarti menggunakan HTTPS (dengan sertifikat yang di-generate secara otomatis, misal menggunakan Let's Encrypt untuk versi web) untuk mencegah serangan man-in-the-middle, dan memastikan token acak yang dihasilkan cukup panjang dan kompleks.
4.	Pembersihan Sumber Daya yang Andal: Aplikasi harus menjamin bahwa file sementara dan server benar-benar dimatikan setelah digunakan, bahkan jika terjadi error atau aplikasi ditutup paksa. Menggunakan defer statement di Go untuk operasi pembersihan dan menangani sinyal interupsi dari sistem operasi (SIGINT) adalah kunci untuk mencapai ini.
________________________________________
Ini adalah detail untuk share-it. Proyek yang paling sederhana dalam daftar kita, tetapi tetap membutuhkan rekayasa yang cermat untuk membuatnya benar-benar andal dan aman.
