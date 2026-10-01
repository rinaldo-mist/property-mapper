# Facility Map — Sedayu, JGC, KHI, dan Metland

Paket ini adalah website statis tanpa framework. Isinya dapat dipasang pada layanan hosting yang menerima file HTML, CSS, JavaScript, dan aset biasa.

## Cara memasang

1. Ekstrak ZIP.
2. Unggah seluruh isi folder ini dengan struktur yang tetap sama.
3. Jadikan `index.html` sebagai halaman utama.
4. Buka melalui alamat HTTP/HTTPS. Jangan hanya membuka `index.html` langsung dari komputer (`file://`), karena browser dapat memblokir pembacaan KMZ.

Struktur file:

```text
index.html
styles.css
app.js
data/
  facility-mapping.kmz
```

## Memperbarui data

Ganti `data/facility-mapping.kmz` dengan file KMZ baru dan pertahankan nama filenya, atau
unggah lewat **Unggah KMZ ke server** sebagai admin. Website membaca titik, kategori,
developer, populasi, dan boundary dari berkas tersebut saat halaman dibuka.

### Struktur yang dibaca

Tidak ada nama developer yang ditulis di dalam kode — semuanya berasal dari berkas:

```text
Facilities/
  <Developer>/                 <- nama folder menjadi kunci developer
    Boundary_<...>/            <- Document berisi polygon; nama placemark-nya
      <Placemark polygon>         dipakai sebagai label yang tampil
    <Kategori>/                <- School, Hospital, Showroom Dealer, Gas Station, ...
      <Placemark titik>
Population/
  <Developer>/                 <- kunci yang sama persis
    <Placemark "± 20.000 s/d 29.000 jiwa">
```

Bila sebuah berkas tidak memiliki folder `Facilities`, developer dicari dari folder mana
pun yang memiliki boundary. Bila tidak ada sama sekali — seperti dataset Industrial —
maka berkas itu tidak punya developer, dan itu sah: titiknya tetap terbaca tanpa kawasan.

## Menambah pin manual dan logo

1. Tekan **Tambah pin manual**.
2. Klik lokasi yang diinginkan pada peta.
3. Isi nama, developer, dan kategori.
4. Unggah logo PNG, JPG, WebP, atau SVG jika diperlukan, lalu simpan.
5. Klik pin manual dan pilih **Edit pin / logo** untuk mengubah atau menghapusnya.

## Menghapus fasilitas

Klik titik mana pun pada peta, lalu tekan **Hapus** di dalam popup. Tombol ini hanya
tampil untuk admin, dan berlaku untuk ketiga jenis titik: fasilitas dari KMZ, pin manual,
dan perumahan.

Fasilitas dari KMZ dibentuk ulang dari berkas setiap kali halaman dimuat, sehingga
menghapusnya bukan sekadar membuangnya dari daftar — id-nya **diingat** dan diterapkan
kembali setelah berkas dibaca. Akibatnya:

- Penghapusan tetap berlaku setelah halaman dimuat ulang, dan ikut terkirim ke pengunjung
  lain seperti perubahan lain yang dilakukan admin.
- Saat **Simpan ke KMZ**, placemark aslinya ikut dikeluarkan dari berkas. Berkas hasil
  ekspor benar-benar tidak lagi memuat fasilitas tersebut.
- Selama belum diekspor, panel kiri menampilkan **"N fasilitas KMZ dihapus · Pulihkan"**.
  **Pulihkan** mengembalikan semuanya sekaligus.

Pin manual dan perumahan tidak punya mekanisme ini — keduanya memang milik aplikasi,
sehingga menghapusnya langsung hilang, persis seperti tombol hapus di dalam dialognya.

Mengunggah berkas KMZ baru **mengosongkan** daftar penghapusan: berkas penggantinya sudah
tidak memuat titik-titik itu (bila berasal dari ekspor), atau memang tidak pernah
memuatnya.

Pin manual dan logo disimpan melalui `localStorage`, sehingga tetap tersedia saat halaman dibuka kembali pada browser dan domain yang sama. Untuk membagikannya ke perangkat atau pengunjung lain, gunakan **Simpan ke KMZ** (lihat di bawah).

Boundary menggunakan garis merah lembut dan area light red dengan transparansi 50%.

## Mode: Residential dan Industrial

Tombol di atas daftar developer memilih **dataset** yang sedang dibuka. Keduanya terpisah
sepenuhnya: berkas KMZ sendiri, dan salinan sendiri untuk pin manual, perumahan, populasi,
serta kategori kustom. Berpindah mode tidak pernah mencampur data keduanya.

- **Residential** membaca `data/facility-mapping.kmz`.
- **Industrial** membaca `data/industrial.kmz`.

Jika berkas untuk sebuah mode belum ada, peta tampil kosong dengan keterangan
"Dataset ... belum tersedia" — panel filter tetap dapat dipakai.

**Panel menyesuaikan isi berkas.** Tidak ada daftar filter yang ditetapkan per mode;
setiap grup muncul hanya bila datanya ada:

| Bagian | Muncul bila |
|---|---|
| **Developer** | berkas memiliki folder kawasan |
| **Range Harga** | ada perumahan dengan katalog unit |
| **Populasi** | ada kawasan yang mencantumkan jumlah jiwa |
| **Tipe Simpul** / **Operator** | ada titik Transportasi Umum |
| **Fasilitas** | selalu, dari kategori yang ada di berkas |
| **Tambah perumahan** | berkas memiliki boundary — perumahan wajib berada di dalamnya |

Karena itu dataset Industrial yang datar (tanpa folder kawasan) otomatis tampil tanpa
daftar developer, tanpa Range Harga, dan tanpa Populasi — tanpa pengaturan tambahan.

> Catatan: harga lahan industri biasanya dihitung per m², bukan miliar per unit seperti
> perumahan. Grup **Range Harga** saat ini memakai skala residensial.

### Menambah mode baru

Cukup satu perubahan, pada objek `MODES` di `app.js`:

```js
const MODES = {
  residential: { label: 'Residential', kmz: 'data/facility-mapping.kmz' },
  industrial:  { label: 'Industrial',  kmz: 'data/industrial.kmz' },
  commercial:  { label: 'Commercial',  kmz: 'data/commercial.kmz' }   // <- cukup ini
};
```

Tombol mode, kunci `localStorage`, berkas di server, dan nomor versi semuanya mengikuti.
Sisi server tidak menyimpan daftar mode — ia hanya memastikan namanya aman dipakai sebagai
nama berkas (huruf kecil, angka, tanda hubung), sehingga tidak ada dua daftar yang harus
disamakan.

## Filter

Panel kiri berisi beberapa grup filter yang berdiri sendiri:

- **Fasilitas** — School, Hospital, Showroom, SPBU, University, Gerbang Tol,
  Transportasi Umum, dan Perumahan.
- **Range Harga** — diambil dari katalog unit setiap perumahan (≤ 1 M, 1–2 M, 2–3 M, > 3 M).
- **Populasi** — atribut kawasan (≤ 10rb, 10–30rb, 30–60rb, > 60rb).
- **Tipe Simpul** dan **Operator** — muncul hanya bila ada titik Transportasi Umum.
- Grup buatan sendiri melalui **Kelola kategori**.

Aturannya:

- **Grup tanpa pilihan tidak menyaring apa pun.** Semua chip mati saat halaman dibuka, dan menekan **Reset** mengembalikan satu grup ke keadaan itu.
- Dalam satu grup pilihan bersifat **ATAU**; antar grup bersifat **DAN**.
- Tombol **Multi** mengatur apakah satu grup boleh memilih lebih dari satu chip.
- Titik yang tidak punya nilai pada grup yang sedang menyaring akan disembunyikan. Karena itu memilih salah satu Range Harga menyisakan perumahan saja — fasilitas biasa memang tidak punya harga.

Menekan salah satu developer memfokuskan peta ke kawasan tersebut dengan animasi. Pencarian dan perubahan chip sengaja **tidak** menggeser peta.

## Fasilitas umum

**Gerbang Tol** dan **Transportasi Umum** adalah fasilitas publik, sehingga tidak dimiliki
developer mana pun. Saat salah satu kategori ini dipilih pada dialog pin, kolom
**Developer** otomatis disembunyikan dan titik tersebut disimpan tanpa kawasan — daftar
maupun popup menampilkannya sebagai *Fasilitas umum*.

Karena bukan milik developer, titik ini **tetap terlihat** ketika sebuah developer sedang
dipilih dan ketika grup Populasi sedang menyaring. Keduanya adalah atribut kawasan yang
memang tidak dimiliki fasilitas publik.

### Transportasi Umum

Setiap titik memiliki **tipe simpul** dan **operator**:

| Tipe | Dipakai oleh |
|---|---|
| **Stasiun** | MRT Jakarta, LRT Jakarta, LRT Jabodebek, KRL Commuterline, KA Bandara, Whoosh |
| **Terminal Bus** | DAMRI, bus AKAP/AKDP, bus kota |
| **Halte BRT** | TransJakarta |

Operator bersifat **pilihan ganda**, karena satu titik dapat dilayani beberapa sistem
sekaligus — Stasiun Dukuh Atas misalnya melayani MRT, LRT Jakarta, KRL, dan TransJakarta.
Titik tersebut akan muncul pada keempat chip operator.

Angkot/mikrolet sengaja tidak dimasukkan: rutenya tetap, tetapi tidak memiliki titik henti
tetap untuk dipetakan. Mikrotrans (JakLingko) tersedia karena memakai halte resmi.

## Mencari lokasi

Kotak pencarian menyaring fasilitas pada peta, dan sekaligus mencari alamat melalui
**Photon** (OpenStreetMap) — tanpa API key dan tanpa biaya. Hasil alamat muncul di bagian
**Lokasi** di bawah daftar **Fasilitas**; memilihnya menggeser peta ke titik tersebut dan
tidak membuat pin baru.

Pencarian alamat berjalan setelah jeda ~300 ms dan minimal 3 huruf. Bila jaringan gagal,
pencarian fasilitas lokal tetap berfungsi seperti biasa.

## Menggambar dan mengubah kawasan

Kawasan tidak harus berasal dari berkas KMZ — admin dapat menggambarnya langsung. Istilah
yang dipakai mengikuti mode: **Developer** pada Residential, **Kawasan** pada Industrial.

### Menggambar kawasan baru

1. Tekan **＋ Gambar** di sebelah judul daftar kawasan (hanya tampil untuk admin).
2. Klik tiap sudut pada peta, berurutan mengelilingi kawasan. Minimal tiga sudut sebelum
   dapat disimpan; jumlah titik terlihat pada panel di atas peta.
3. Tekan **Selesai**, beri nama, lalu simpan.

Kawasan hasil gambar berlaku **sama persis** seperti kawasan dari KMZ: muncul di daftar,
pin dapat dimiliki olehnya, klik kanan di dalamnya membuka menu, populasi dapat diisi, dan
perumahan dapat ditempatkan di dalamnya.

### Mengubah batas kawasan

Klik kanan di dalam sebuah kawasan lalu pilih **Ubah batas**. Ini berlaku untuk kawasan
hasil gambar **maupun** kawasan dari KMZ.

- Setiap sudut menjadi titik yang dapat **digeser**.
- Setiap sisi mendapat **titik bayangan** (lingkaran kecil berwarna pudar) di tengahnya.
  Klik titik itu untuk menyisipkan sudut baru tepat pada sisi tersebut — cara paling pasti
  untuk menentukan letak sudut.
- Klik pada peta juga menambah sudut, dan sudut itu disisipkan pada **sisi terdekat**,
  bukan di ujung daftar. Saat menggambar kawasan baru secara berurutan hasilnya tetap
  sama seperti menambah di ujung, karena sisi terdekat dari klik setelah sudut terakhir
  memang sisi penutupnya.
- Klik sebuah sudut untuk menghapusnya (minimal tiga sudut tetap dipertahankan).
- **Batal** mengembalikan batas seperti semula — perubahan hanya tersimpan setelah
  **Selesai** lalu **Simpan**.

Titik yang berada di luar layar tidak digambar sebagai pegangan: boundary dari KMZ bisa
berisi ratusan sudut, dan menampilkan semuanya sekaligus membuat peta berat. Geser atau
perbesar peta untuk memunculkannya kembali. Titik bayangan juga disembunyikan pada sisi
yang terlalu pendek di layar — perbesar peta dan titiknya muncul.

Kawasan hasil gambar dapat dihapus. Kawasan dari KMZ yang diubah **tidak** dapat dihapus,
karena berkas KMZ akan menyediakannya lagi saat halaman dimuat ulang; yang tersimpan hanya
perubahan batasnya.

### Ekspor

Kawasan ditulis ke KMZ dalam struktur `Facilities/<nama>/Boundary_<nama>/` — bentuk yang
sama seperti berkas aslinya. Artinya berkas hasil ekspor adalah dataset yang sah: dapat
dibuka di Google Earth, dan dapat diunggah kembali sebagai sumber data.

Saat mengubah kawasan dari KMZ, hanya bagian boundary yang ditulis ulang; folder kategori
beserta seluruh titik fasilitas di dalamnya tetap utuh.

Mode yang belum memiliki berkas KMZ sama sekali tetap dapat diekspor — dokumen KML dibuat
dari awal. Jadi dataset Industrial bisa dibangun sepenuhnya di dalam aplikasi.

## Perumahan dan populasi

### Menambah perumahan

1. Tekan **＋ Tambah perumahan** pada panel kiri.
2. Klik lokasi di peta. **Lokasi harus berada di dalam boundary salah satu kawasan** — klik di luar boundary akan ditolak dan mode pemilihan tetap aktif. Developer terisi otomatis mengikuti kawasan yang terpilih.
3. Isi nama dan katalog unit (LT, LB, Harga dalam miliar). Minimal satu baris katalog harus terisi lengkap dan lebih besar dari nol sebelum dapat disimpan.

Perumahan muncul sebagai pin hijau, ikut tersaring pada Range Harga sesuai rentang katalognya, dan dapat diubah lewat **Edit perumahan** pada popup pin.

Sebagai jalan pintas, **klik kanan di dalam boundary** sebuah kawasan juga membuka menu berisi **Tambah perumahan** dan **Set populasi**.

### Mengubah populasi

Arahkan kursor ke salah satu kartu developer, lalu tekan ikon populasi di sudut kanan atas kartu. Nilai asal dari KMZ dapat dikembalikan kapan saja lewat **Kembalikan dari KMZ**.

## Menyimpan ke KMZ

Semua perubahan langsung tersimpan di `localStorage`. Tombol **Simpan ke KMZ** menuliskannya kembali ke berkas KMZ; titik oranye pada tombol menandakan ada perubahan yang belum disimpan.

- Di Chrome dan Edge berkas asli ditimpa langsung setelah satu kali izin diberikan.
- Di Firefox dan Safari berkas diunduh, lalu salin sendiri ke folder `data/`.

Data aplikasi ditulis di dalam satu folder `Property Mapper` beserta `ExtendedData` berawalan `pm:`, sehingga struktur, gaya, dan deskripsi asli dari Google Earth tetap utuh dan berkas tetap dapat dibuka di Google Earth.

## Admin dan pengunjung

Tanpa server, kata sandi apa pun yang ditaruh di berkas JavaScript dapat dibaca siapa saja
melalui devtools — termasuk nilai dari environment variable, karena ikut tertanam saat
build. Karena itu pemeriksaan admin dilakukan di sisi server melalui folder `api/`.

- **Pengunjung** melihat peta dalam mode baca saja. Semua tombol pengubah data
  disembunyikan, termasuk klik kanan pada boundary.
- **Admin** masuk melalui **Masuk sebagai admin**, lalu dapat mengubah data dan
  mengunggah KMZ ke server.

Perubahan admin tersimpan ke server dan terlihat oleh pengunjung lain **dalam ~1 menit**.

Batas itu berasal dari Vercel Blob: menimpa berkas pada path yang sama membutuhkan waktu
hingga 60 detik untuk menyebar melalui CDN mereka. Menambah parameter unik pada URL hanya
mengatasi cache *browser*, bukan penyebaran CDN tersebut, sehingga memperpendek interval
polling saja tidak membuatnya lebih cepat. Halaman memeriksa versi setiap 30 detik, dan
hanya ketika tab sedang aktif.

Bila suatu saat diperlukan pembaruan yang jauh lebih cepat (~10 detik), caranya adalah
menulis ke path baru setiap versi — Vercel menganjurkan memperlakukan blob sebagai
*immutable* — lalu menanyakan versi terkini lewat sebuah Function, bukan lewat blob yang
ter-cache. Konsekuensinya: satu pemanggilan Function per pengunjung per polling.

### Menyiapkan server

1. Buat **Blob store** pada proyek Vercel (Storage → Create → Blob). Vercel mengisi
   `BLOB_READ_WRITE_TOKEN` secara otomatis.
2. Isi dua environment variable pada proyek:

   ```bash
   openssl rand -base64 48        # -> SESSION_SECRET
   npm run hash -- "kata sandi"   # -> ADMIN_PASSWORD_HASH
   ```

   Hanya *hash* yang disimpan; kata sandi aslinya tidak pernah dikirim ke mana pun.
3. Deploy. `npm install` dijalankan Vercel secara otomatis.

Bila `api/` belum ter-deploy, halaman tetap berjalan seperti situs statis biasa: data
dibaca dari `data/*.kmz` dan perubahan tersimpan di `localStorage` browser masing-masing.

### Catatan

Hanya ada satu akun admin. Bila dua admin menyimpan pada saat bersamaan, perubahan terakhir
yang menang untuk data, sementara nomor versi memakai compare-and-set sehingga tidak ada
pembaruan yang hilang diam-diam.

## Platform yang cocok

- GitHub Pages, Netlify, Cloudflare Pages, Vercel static hosting
- cPanel/shared hosting
- WordPress self-hosted dengan akses file hosting

Jika platform hanya menerima potongan HTML (misalnya beberapa page builder), unggah paket ini ke static hosting lalu sematkan URL-nya memakai `iframe`.

## Dependensi internet

Halaman memakai Leaflet, JSZip, Google Fonts, dan tile OpenStreetMap dari CDN. Karena itu, koneksi internet diperlukan saat halaman dibuka.
