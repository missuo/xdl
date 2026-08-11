#import "XDLMainWindowController.h"
#import "XDLCoreClient.h"
#import "XDLSidebarViewController.h"
#import "XDLTheme.h"
#import "XDLFetchViewController.h"
#import "XDLLibraryViewController.h"
#import "XDLGifsViewController.h"
#import "XDLDependenciesViewController.h"
#import "XDLLogsViewController.h"
#import "XDLSettingsViewController.h"
#import "XDLAboutViewController.h"
#import "XDLPageViewController.h"

static const CGFloat kWindowWidth = 980.0;
static const CGFloat kWindowHeight = 780.0;

/// One diagonal gradient across the whole window, sidebar included.
@interface XDLGradientView : NSView
@end

@implementation XDLGradientView

- (BOOL)isOpaque { return YES; }

- (void)drawRect:(NSRect)dirtyRect {
    NSArray<NSColor *> *colors = XDLTheme.backdropColors;
    NSGradient *gradient = [[NSGradient alloc] initWithColorsAndLocations:
        colors[0], 0.0, colors[1], 0.5, colors[2], 1.0, nil];
    [gradient drawInRect:self.bounds angle:315.0];
}

- (void)viewDidChangeEffectiveAppearance {
    [super viewDidChangeEffectiveAppearance];
    self.needsDisplay = YES;
}

@end

@interface XDLMainWindowController ()
@property (nonatomic) XDLCoreClient *client;
@property (nonatomic) XDLSidebarViewController *sidebar;
@property (nonatomic) NSView *contentContainer;
@property (nonatomic) NSMutableDictionary<NSString *, NSViewController *> *pages;
@property (nonatomic, nullable) NSViewController *currentPage;
@end

@implementation XDLMainWindowController

- (instancetype)initWithCoreClient:(XDLCoreClient *)client {
    NSWindow *window = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, kWindowWidth, kWindowHeight)
                  styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                            NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable |
                            NSWindowStyleMaskFullSizeContentView
                    backing:NSBackingStoreBuffered
                      defer:NO];
    window.titlebarAppearsTransparent = YES;
    window.titleVisibility = NSWindowTitleHidden;
    window.movableByWindowBackground = YES;
    window.title = @"XDL";
    // Growable, but never smaller than the designed layout.
    window.contentMinSize = NSMakeSize(kWindowWidth, kWindowHeight);
    [window center];

    self = [super initWithWindow:window];
    if (self) {
        _client = client;
        _pages = [NSMutableDictionary dictionary];
        [self buildContent];
    }
    return self;
}

- (void)buildContent {
    XDLGradientView *root = [[XDLGradientView alloc] initWithFrame:NSMakeRect(0, 0, kWindowWidth, kWindowHeight)];
    self.window.contentView = root;

    self.sidebar = [[XDLSidebarViewController alloc] init];
    self.sidebar.view.translatesAutoresizingMaskIntoConstraints = NO;
    [root addSubview:self.sidebar.view];

    self.contentContainer = [[NSView alloc] initWithFrame:NSZeroRect];
    self.contentContainer.translatesAutoresizingMaskIntoConstraints = NO;
    [root addSubview:self.contentContainer];

    [NSLayoutConstraint activateConstraints:@[
        [self.sidebar.view.topAnchor constraintEqualToAnchor:root.topAnchor],
        [self.sidebar.view.leadingAnchor constraintEqualToAnchor:root.leadingAnchor],
        [self.sidebar.view.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],
        [self.sidebar.view.widthAnchor constraintEqualToConstant:XDLSidebarWidth],

        [self.contentContainer.topAnchor constraintEqualToAnchor:root.topAnchor],
        [self.contentContainer.leadingAnchor constraintEqualToAnchor:self.sidebar.view.trailingAnchor],
        [self.contentContainer.trailingAnchor constraintEqualToAnchor:root.trailingAnchor],
        [self.contentContainer.bottomAnchor constraintEqualToAnchor:root.bottomAnchor],
    ]];

    __weak typeof(self) weakSelf = self;
    self.sidebar.onSelect = ^(NSString *itemID) {
        [weakSelf displayPage:itemID];
    };
    [self.sidebar selectItemID:@"fetch"];
}

- (void)showPage:(NSString *)pageID {
    [self.sidebar selectItemID:pageID];
}

- (void)displayPage:(NSString *)pageID {
    NSViewController *page = self.pages[pageID];
    if (!page) {
        page = [self makePageFor:pageID];
        self.pages[pageID] = page;
    }
    if (page == self.currentPage) {
        return;
    }
    [self.currentPage.view removeFromSuperview];
    self.currentPage = page;

    page.view.translatesAutoresizingMaskIntoConstraints = NO;
    [self.contentContainer addSubview:page.view];
    [NSLayoutConstraint activateConstraints:@[
        [page.view.topAnchor constraintEqualToAnchor:self.contentContainer.topAnchor],
        [page.view.leadingAnchor constraintEqualToAnchor:self.contentContainer.leadingAnchor],
        [page.view.trailingAnchor constraintEqualToAnchor:self.contentContainer.trailingAnchor],
        [page.view.bottomAnchor constraintEqualToAnchor:self.contentContainer.bottomAnchor],
    ]];
}

- (NSViewController *)makePageFor:(NSString *)pageID {
    if ([pageID isEqualToString:@"fetch"]) {
        return [[XDLFetchViewController alloc] initWithClient:self.client];
    }
    if ([pageID isEqualToString:@"library"]) {
        XDLLibraryViewController *library = [[XDLLibraryViewController alloc] initWithClient:self.client];
        __weak typeof(self) weakSelf = self;
        library.onOpenAccount = ^(NSString *username, NSString *mediaType) {
            [weakSelf showPage:@"fetch"];
            XDLFetchViewController *fetch = (XDLFetchViewController *)weakSelf.pages[@"fetch"];
            [fetch loadSavedAccount:username mediaType:mediaType];
        };
        library.onRefetchAccount = ^(NSString *username, NSString *mediaType) {
            [weakSelf showPage:@"fetch"];
            XDLFetchViewController *fetch = (XDLFetchViewController *)weakSelf.pages[@"fetch"];
            [fetch refetchAccount:username mediaType:mediaType];
        };
        return library;
    }
    if ([pageID isEqualToString:@"gifs"]) {
        return [[XDLGifsViewController alloc] initWithClient:self.client];
    }
    if ([pageID isEqualToString:@"dependencies"]) {
        return [[XDLDependenciesViewController alloc] initWithClient:self.client];
    }
    if ([pageID isEqualToString:@"logs"]) {
        return [[XDLLogsViewController alloc] initWithClient:self.client];
    }
    if ([pageID isEqualToString:@"settings"]) {
        return [[XDLSettingsViewController alloc] initWithClient:self.client];
    }
    if ([pageID isEqualToString:@"about"]) {
        return [[XDLAboutViewController alloc] init];
    }
    return [[XDLPageViewController alloc] initWithTitle:pageID.capitalizedString];
}

@end
