#import "XDLDependenciesViewController.h"
#import "XDLCoreClient.h"
#import "XDLComponents.h"
#import "XDLTheme.h"

@interface XDLDepRowViews : NSObject
@property (nonatomic) NSImageView *statusIcon;
@property (nonatomic) NSTextField *statusLabel;
@property (nonatomic) XDLPillButton *downloadButton;
@property (nonatomic) NSProgressIndicator *progress;
@end

@implementation XDLDepRowViews
@end

@interface XDLDependenciesViewController ()
@property (nonatomic) XDLCoreClient *client;
@property (nonatomic) NSDictionary<NSString *, XDLDepRowViews *> *rows;
@end

@implementation XDLDependenciesViewController

- (instancetype)initWithClient:(XDLCoreClient *)client {
    self = [super initWithTitle:@"Dependencies"];
    if (self) {
        _client = client;
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];

    NSMutableDictionary *rows = [NSMutableDictionary dictionary];
    NSLayoutYAxisAnchor *anchor = self.contentTopAnchor;
    CGFloat gap = 6;

    struct {
        const char *key;
        const char *caption;
        const char *blurb;
    } deps[] = {
        { "xtractor", "Core — Xtractor", "Extracts timelines from x.com. Required for fetching." },
        { "ffmpeg", "GIF Conversion — FFmpeg", "Converts animated GIF mp4s to real .gif files. Optional." },
        { "exiftool", "Metadata — ExifTool", "Embeds the tweet URL into downloaded jpg/mp4 files. Optional." },
    };

    for (size_t i = 0; i < sizeof(deps) / sizeof(deps[0]); i++) {
        NSString *key = @(deps[i].key);
        NSView *card = nil;
        NSStackView *stack = [self addCardWithCaption:@(deps[i].caption)
                                          belowAnchor:anchor
                                                  gap:gap
                                              outCard:&card];
        gap = 18;
        anchor = card.bottomAnchor;

        NSTextField *blurb = [XDLUI labelWithText:@(deps[i].blurb)
                                             size:11.5
                                           weight:NSFontWeightRegular
                                            color:XDLTheme.secondaryLabel];
        [stack addArrangedSubview:blurb];

        XDLDepRowViews *views = [[XDLDepRowViews alloc] init];

        views.statusIcon = [[NSImageView alloc] initWithFrame:NSZeroRect];
        views.statusIcon.translatesAutoresizingMaskIntoConstraints = NO;
        [NSLayoutConstraint activateConstraints:@[
            [views.statusIcon.widthAnchor constraintEqualToConstant:18],
            [views.statusIcon.heightAnchor constraintEqualToConstant:18],
        ]];

        views.statusLabel = [XDLUI labelWithText:@"Checking…" size:12.5 weight:NSFontWeightRegular color:XDLTheme.label];
        // The label absorbs the leftover width so the button hugs the right edge.
        [views.statusLabel setContentHuggingPriority:100
                                      forOrientation:NSLayoutConstraintOrientationHorizontal];
        __weak typeof(self) weakSelf = self;
        views.downloadButton = [XDLPillButton pillWithTitle:@"Download" onClick:^{
            [weakSelf downloadDependency:key];
        }];
        views.downloadButton.enabled = NO;
        views.progress = [[NSProgressIndicator alloc] initWithFrame:NSZeroRect];
        views.progress.style = NSProgressIndicatorStyleBar;
        views.progress.indeterminate = NO;
        views.progress.minValue = 0;
        views.progress.maxValue = 1;
        views.progress.translatesAutoresizingMaskIntoConstraints = NO;
        views.progress.hidden = YES;

        NSStackView *row = [NSStackView stackViewWithViews:@[
            views.statusIcon, views.statusLabel, views.progress, views.downloadButton
        ]];
        row.spacing = 12;
        row.distribution = NSStackViewDistributionFill;
        [stack addArrangedSubview:row];
        [NSLayoutConstraint activateConstraints:@[
            [row.widthAnchor constraintEqualToAnchor:stack.widthAnchor],
            [views.progress.widthAnchor constraintEqualToConstant:180],
        ]];
        [self setIcon:views symbol:@"circle.dashed" color:XDLTheme.secondaryLabel];
        rows[key] = views;
    }
    self.rows = rows;

    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(coreEvent:)
                                               name:XDLCoreEventNotification
                                             object:nil];
    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(refreshStatus)
                                               name:XDLCoreStateDidChangeNotification
                                             object:nil];
    [self refreshStatus];
}

- (void)dealloc {
    [NSNotificationCenter.defaultCenter removeObserver:self];
}

- (void)viewDidAppear {
    [super viewDidAppear];
    [self refreshStatus];
}

- (void)setIcon:(XDLDepRowViews *)views symbol:(NSString *)symbol color:(NSColor *)color {
    NSImage *image = [NSImage imageWithSystemSymbolName:symbol accessibilityDescription:nil];
    NSImageSymbolConfiguration *config =
        [NSImageSymbolConfiguration configurationWithPointSize:15 weight:NSFontWeightSemibold];
    views.statusIcon.image = [image imageWithSymbolConfiguration:config];
    views.statusIcon.contentTintColor = color;
}

- (void)refreshStatus {
    if (self.client.state != XDLCoreStateReady) {
        return;
    }
    __weak typeof(self) weakSelf = self;
    [self.client get:@"/deps/status" completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;
        if (error || ![json isKindOfClass:NSDictionary.class]) {
            for (XDLDepRowViews *views in self.rows.allValues) {
                views.statusLabel.stringValue = error.localizedDescription ?: @"Status unavailable";
            }
            return;
        }
        [self.rows enumerateKeysAndObjectsUsingBlock:^(NSString *key, XDLDepRowViews *views, BOOL *stop) {
            NSDictionary *status = json[key];
            if (![status isKindOfClass:NSDictionary.class]) {
                views.statusLabel.stringValue = @"Unknown";
                return;
            }
            BOOL installed = [status[@"installed"] boolValue];
            BOOL system = [status[@"system"] boolValue];
            NSString *installedVersion = [status[@"installed_version"] isKindOfClass:NSString.class] ? status[@"installed_version"] : @"";
            NSString *latestVersion = [status[@"latest_version"] isKindOfClass:NSString.class] ? status[@"latest_version"] : @"";
            if (installed && system) {
                // Managed by Homebrew/the OS — nothing for our Download button to do.
                [self setIcon:views symbol:@"checkmark.circle.fill" color:XDLTheme.statusGreen];
                views.statusLabel.stringValue = installedVersion.length
                    ? [NSString stringWithFormat:@"Installed %@ (system)", installedVersion]
                    : @"Installed (system)";
                views.statusLabel.textColor = XDLTheme.label;
                views.downloadButton.hidden = YES;
                return;
            }
            views.downloadButton.hidden = NO;
            if (installed) {
                BOOL outdated = latestVersion.length > 0 && installedVersion.length > 0 &&
                                ![installedVersion isEqualToString:latestVersion];
                if (outdated) {
                    [self setIcon:views symbol:@"exclamationmark.circle.fill" color:XDLTheme.statusOrange];
                    views.statusLabel.stringValue = [NSString stringWithFormat:@"Installed %@ — update %@ available",
                                                     installedVersion, latestVersion];
                } else {
                    [self setIcon:views symbol:@"checkmark.circle.fill" color:XDLTheme.statusGreen];
                    views.statusLabel.stringValue = installedVersion.length
                        ? [NSString stringWithFormat:@"Installed %@", installedVersion]
                        : @"Installed";
                }
                views.statusLabel.textColor = XDLTheme.label;
                views.downloadButton.title = outdated ? @"Update" : @"Reinstall";
            } else {
                [self setIcon:views symbol:@"xmark.circle.fill" color:XDLTheme.statusRed];
                views.statusLabel.stringValue = latestVersion.length
                    ? [NSString stringWithFormat:@"Not installed — latest %@", latestVersion]
                    : @"Not installed";
                views.statusLabel.textColor = XDLTheme.label;
                views.downloadButton.title = @"Download";
            }
            views.downloadButton.enabled = YES;
        }];
    }];
}

- (void)downloadDependency:(NSString *)name {
    XDLDepRowViews *views = self.rows[name];
    views.downloadButton.enabled = NO;
    views.progress.hidden = NO;
    views.progress.doubleValue = 0;
    [self setIcon:views symbol:@"arrow.down.circle" color:XDLTheme.accentBlue];
    views.statusLabel.stringValue = @"Downloading…";
    views.statusLabel.textColor = XDLTheme.label;
    __weak typeof(self) weakSelf = self;
    [self.client post:@"/deps/download" body:@{ @"name": name } completion:^(id json, NSError *error) {
        if (error) {
            XDLDepRowViews *views = weakSelf.rows[name];
            views.statusLabel.stringValue = error.localizedDescription;
            views.downloadButton.enabled = YES;
            views.progress.hidden = YES;
        }
    }];
}

- (void)coreEvent:(NSNotification *)notification {
    NSString *type = notification.userInfo[@"type"];
    id data = notification.userInfo[@"data"];
    if (![data isKindOfClass:NSDictionary.class]) {
        return;
    }
    NSString *name = data[@"name"];
    XDLDepRowViews *views = [name isKindOfClass:NSString.class] ? self.rows[name] : nil;
    if (!views) {
        return;
    }

    if ([type isEqualToString:@"dep-progress"]) {
        double downloaded = [data[@"downloaded"] doubleValue];
        double total = [data[@"total"] doubleValue];
        views.progress.hidden = NO;
        if (total > 0) {
            views.progress.indeterminate = NO;
            views.progress.maxValue = total;
            views.progress.doubleValue = downloaded;
            views.statusLabel.stringValue = [NSString stringWithFormat:@"Downloading… %.0f%%", downloaded * 100.0 / total];
        }
    } else if ([type isEqualToString:@"dep-finished"]) {
        views.progress.hidden = YES;
        NSString *errorMessage = data[@"error"];
        if ([errorMessage isKindOfClass:NSString.class] && errorMessage.length > 0) {
            [self setIcon:views symbol:@"xmark.circle.fill" color:XDLTheme.statusRed];
            views.statusLabel.stringValue = errorMessage;
            views.statusLabel.textColor = XDLTheme.statusRed;
            views.downloadButton.enabled = YES;
        } else {
            [self refreshStatus];
        }
    }
}

@end
