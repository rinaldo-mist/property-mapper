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

Ganti `data/facility-mapping.kmz` dengan file KMZ baru dan pertahankan nama filenya. Website akan membaca titik, kategori, developer, serta boundary dari KMZ saat halaman dibuka.

## Menambah pin manual dan logo

1. Tekan **Tambah pin manual**.
2. Klik lokasi yang diinginkan pada peta.
3. Isi nama, developer, dan kategori.
4. Unggah logo PNG, JPG, WebP, atau SVG jika diperlukan, lalu simpan.
5. Klik pin manual dan pilih **Edit pin / logo** untuk mengubah atau menghapusnya.

Pin manual dan logo disimpan melalui `localStorage`, sehingga tetap tersedia saat halaman dibuka kembali pada browser dan domain yang sama. Untuk membagikannya ke perangkat atau pengunjung lain, gunakan **Simpan ke KMZ** (lihat di bawah).

Boundary menggunakan garis merah lembut dan area light red dengan transparansi 50%.

## Filter

Panel kiri berisi beberapa grup filter yang berdiri sendiri:

- **Fasilitas** — School, Hospital, Showroom, SPBU, University, dan Perumahan.
- **Range Harga** — diambil dari katalog unit setiap perumahan (≤ 1 M, 1–2 M, 2–3 M, > 3 M).
- **Populasi** — atribut kawasan (≤ 10rb, 10–30rb, 30–60rb, > 60rb).
- Grup buatan sendiri melalui **Kelola kategori**.

Aturannya:

- **Grup tanpa pilihan tidak menyaring apa pun.** Semua chip mati saat halaman dibuka, dan menekan **Reset** mengembalikan satu grup ke keadaan itu.
- Dalam satu grup pilihan bersifat **ATAU**; antar grup bersifat **DAN**.
- Tombol **Multi** mengatur apakah satu grup boleh memilih lebih dari satu chip.
- Titik yang tidak punya nilai pada grup yang sedang menyaring akan disembunyikan. Karena itu memilih salah satu Range Harga menyisakan perumahan saja — fasilitas biasa memang tidak punya harga.

Menekan salah satu developer memfokuskan peta ke kawasan tersebut dengan animasi. Pencarian dan perubahan chip sengaja **tidak** menggeser peta.

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

## Platform yang cocok

- GitHub Pages, Netlify, Cloudflare Pages, Vercel static hosting
- cPanel/shared hosting
- WordPress self-hosted dengan akses file hosting

Jika platform hanya menerima potongan HTML (misalnya beberapa page builder), unggah paket ini ke static hosting lalu sematkan URL-nya memakai `iframe`.

## Dependensi internet

Halaman memakai Leaflet, JSZip, Google Fonts, dan tile OpenStreetMap dari CDN. Karena itu, koneksi internet diperlukan saat halaman dibuka.
