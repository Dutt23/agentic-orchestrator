/// Mover Service - Ultra-fast data mover with io_uring and zero-copy
///
/// Provides low-level primitives for Go services:
/// - READ: Zero-copy reads from mmap'd CAS
/// - WRITE: Write-through to CAS
/// - SEND_ZC: Zero-copy network send
/// - RECV: Receive into registered buffers
///
/// Communication: Unix Domain Socket
/// I/O: io_uring for all operations
/// Storage: Memory-mapped CAS files

// Linux implementation
#[cfg(target_os = "linux")]
fn main() -> anyhow::Result<()> {
    use mover::server::run_mover;
    use tracing::info;

    // Initialize tracing first
    tracing_subscriber::fmt()
        .with_target(false)
        .with_thread_ids(true)
        .init();

    info!("===========================================");
    info!(" Mover Service - Monoio (io_uring)");
    info!("===========================================");

    // Create monoio runtime (pure io_uring with true zero-copy support!)
    let mut runtime = monoio::RuntimeBuilder::<monoio::FusionDriver>::new()
        .enable_all()
        .build()
        .expect("Failed to build monoio runtime");

    runtime.block_on(async {
        if let Err(e) = run_mover().await {
            eprintln!("FATAL: Mover failed: {}", e);
            std::process::exit(1);
        }
    });

    Ok(())
}

// Non-Linux stub - exits with error message
#[cfg(not(target_os = "linux"))]
fn main() {
    eprintln!("========================================");
    eprintln!("ERROR: Mover requires Linux");
    eprintln!("========================================");
    eprintln!();
    eprintln!("The mover service uses io_uring and splice syscalls");
    eprintln!("which are only available on Linux (kernel 5.1+).");
    eprintln!();
    eprintln!("For development on macOS/Windows:");
    eprintln!("  1. Use Docker to run mover in a Linux container");
    eprintln!("  2. Set USE_MOVER=false to disable mover");
    eprintln!();
    std::process::exit(1);
}
