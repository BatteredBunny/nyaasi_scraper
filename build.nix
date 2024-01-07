{buildGoModule}:
buildGoModule {
  src = ./.;

  name = "github.com/BatteredBunny/nyaasi_scraper";
  vendorHash = "sha256-HR2fhQ+3IvydezLYFzgmr0adJnAk3PCpPZM9i9yBqkI=";

  ldflags = [
    "-s"
    "-w"
  ];
}
