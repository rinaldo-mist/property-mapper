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

Pin manual dan logo disimpan melalui `localStorage`, sehingga tetap tersedia saat halaman dibuka kembali pada browser dan domain yang sama. Data ini belum tersinkron otomatis ke perangkat atau pengunjung lain; untuk pemakaian bersama diperlukan database atau file data yang dipublikasikan ulang.

Boundary menggunakan garis merah lembut dan area light red dengan transparansi 50%.

## Platform yang cocok

- GitHub Pages, Netlify, Cloudflare Pages, Vercel static hosting
- cPanel/shared hosting
- WordPress self-hosted dengan akses file hosting

Jika platform hanya menerima potongan HTML (misalnya beberapa page builder), unggah paket ini ke static hosting lalu sematkan URL-nya memakai `iframe`.

## Dependensi internet

Halaman memakai Leaflet, JSZip, Google Fonts, dan tile OpenStreetMap dari CDN. Karena itu, koneksi internet diperlukan saat halaman dibuka.
