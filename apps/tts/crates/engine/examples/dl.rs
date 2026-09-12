fn main() {
    for url in std::env::args().skip(1) {
        match tts_engine::weights::download_if_necessary(&url) {
            Ok(p) => println!("ok  {url} -> {}", p.display()),
            Err(e) => println!("ERR {url}: {e:?}"),
        }
    }
}
