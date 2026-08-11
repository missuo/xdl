#import "XDLAppDelegate.h"
#import <UserNotifications/UserNotifications.h>
#import "XDLCoreClient.h"
#import "XDLMainWindowController.h"
#import "XDLSettings.h"

@interface XDLAppDelegate () <UNUserNotificationCenterDelegate, NSMenuDelegate>
@property (nonatomic) XDLCoreClient *coreClient;
@property (nonatomic) XDLMainWindowController *mainWindowController;
@property (nonatomic) NSStatusItem *statusItem;
@property (nonatomic) BOOL notificationsAllowed;
@property (nonatomic) BOOL useLegacyNotifications;
@property (nonatomic) BOOL settingsSyncScheduled;
@property (nonatomic) NSInteger activeBrowserJobs;
@end

@implementation XDLAppDelegate

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    [self setupMainMenu];

    self.coreClient = [[XDLCoreClient alloc] init];
    [self.coreClient start];

    self.mainWindowController = [[XDLMainWindowController alloc] initWithCoreClient:self.coreClient];
    [self.mainWindowController showWindow:nil];
    [NSApp activateIgnoringOtherApps:YES];

    [self setupStatusItem];
    [self requestNotificationPermission];
    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(coreStateChanged:)
                                               name:XDLCoreStateDidChangeNotification
                                             object:nil];
    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(settingsChanged:)
                                               name:XDLSettingsChangedNotification
                                             object:nil];
    [NSNotificationCenter.defaultCenter addObserver:self
                                           selector:@selector(coreEvent:)
                                               name:XDLCoreEventNotification
                                             object:nil];
}

#pragma mark - Settings sync (browser-initiated jobs use these)

- (void)coreStateChanged:(NSNotification *)notification {
    if (self.coreClient.state == XDLCoreStateReady) {
        [self syncSettingsToCore];
    }
}

- (void)settingsChanged:(NSNotification *)notification {
    if (self.settingsSyncScheduled) {
        return;
    }
    self.settingsSyncScheduled = YES;
    __weak typeof(self) weakSelf = self;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.5 * NSEC_PER_SEC)),
                   dispatch_get_main_queue(), ^{
        weakSelf.settingsSyncScheduled = NO;
        [weakSelf syncSettingsToCore];
    });
}

- (void)syncSettingsToCore {
    if (self.coreClient.state != XDLCoreStateReady) {
        return;
    }
    [self.coreClient post:@"/settings" body:[XDLSettings.shared coreSyncPayload] completion:nil];
}

#pragma mark - Notifications for browser-initiated jobs

- (void)requestNotificationPermission {
    // UNUserNotificationCenter throws for processes without a proper code
    // signature/bundle identity (common for unsigned dev builds) — fall back
    // to the legacy NSUserNotification API, which needs no authorization.
    @try {
        __weak typeof(self) weakSelf = self;
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        center.delegate = self;
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                              completionHandler:^(BOOL granted, NSError *error) {
            dispatch_async(dispatch_get_main_queue(), ^{
                weakSelf.notificationsAllowed = granted;
                if (error || !granted) {
                    weakSelf.useLegacyNotifications = YES;
                }
                [weakSelf.coreClient logAppLine:[NSString stringWithFormat:
                    @"notification permission: granted=%d error=%@", granted, error.localizedDescription ?: @"none"]];
            });
        }];
    } @catch (NSException *exception) {
        self.useLegacyNotifications = YES;
        [self.coreClient logAppLine:[NSString stringWithFormat:
            @"UNUserNotificationCenter unavailable (%@), using legacy notifications", exception.name]];
    }
}

// Show banners even while XDL itself is frontmost.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionSound);
}

- (void)postNotificationWithTitle:(NSString *)title body:(NSString *)body {
    if (self.notificationsAllowed && !self.useLegacyNotifications) {
        @try {
            UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
            content.title = title;
            content.body = body;
            content.sound = UNNotificationSound.defaultSound;
            UNNotificationRequest *request =
                [UNNotificationRequest requestWithIdentifier:NSUUID.UUID.UUIDString
                                                     content:content
                                                     trigger:nil];
            __weak typeof(self) weakSelf = self;
            [[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:request
                                                                   withCompletionHandler:^(NSError *error) {
                if (error) {
                    dispatch_async(dispatch_get_main_queue(), ^{
                        weakSelf.useLegacyNotifications = YES;
                        [weakSelf postLegacyNotificationWithTitle:title body:body];
                    });
                }
            }];
            return;
        } @catch (NSException *exception) {
            self.useLegacyNotifications = YES;
        }
    }
    [self postLegacyNotificationWithTitle:title body:body];
}

// Unsigned dev builds can't register with usernoted (UNErrorDomain error 1),
// so deliver through osascript instead — it is Apple-signed, needs no
// permission, and always shows a banner.
- (void)postLegacyNotificationWithTitle:(NSString *)title body:(NSString *)body {
    NSString *(^escape)(NSString *) = ^(NSString *text) {
        NSString *escaped = [text stringByReplacingOccurrencesOfString:@"\\" withString:@"\\\\"];
        return [escaped stringByReplacingOccurrencesOfString:@"\"" withString:@"\\\""];
    };
    NSString *script = [NSString stringWithFormat:
        @"display notification \"%@\" with title \"%@\" sound name \"Glass\"",
        escape(body ?: @""), escape(title ?: @"XDL")];

    NSTask *task = [[NSTask alloc] init];
    task.executableURL = [NSURL fileURLWithPath:@"/usr/bin/osascript"];
    task.arguments = @[ @"-e", script ];
    NSError *error = nil;
    if (![task launchAndReturnError:&error]) {
        [self.coreClient logAppLine:[NSString stringWithFormat:
            @"osascript notification failed: %@", error.localizedDescription]];
    } else {
        [self.coreClient logAppLine:[NSString stringWithFormat:@"notification sent (osascript): %@", title]];
    }
}

// Dock feedback needs no permission at all: a "↓" badge while browser jobs
// run, one dock bounce when a job finishes and XDL is in the background.
- (void)updateDockBadge {
    NSApp.dockTile.badgeLabel = self.activeBrowserJobs > 0 ? @"↓" : @"";
    [self applyStatusItemIcon];
}

- (void)coreEvent:(NSNotification *)notification {
    if (![notification.userInfo[@"type"] isEqualToString:@"browser-job"]) {
        return;
    }
    id data = notification.userInfo[@"data"];
    if (![data isKindOfClass:NSDictionary.class]) {
        return;
    }
    NSString *stage = data[@"stage"];
    NSString *username = [data[@"username"] isKindOfClass:NSString.class] ? data[@"username"] : @"";

    if ([stage isEqualToString:@"queued"]) {
        self.activeBrowserJobs += 1;
        [self updateDockBadge];
        [self.coreClient logAppLine:[NSString stringWithFormat:@"browser job queued: @%@", username]];
        [self postNotificationWithTitle:@"XDL"
                                   body:[NSString stringWithFormat:@"Queued @%@ from the browser", username]];
    } else if ([stage isEqualToString:@"done"] || [stage isEqualToString:@"error"]) {
        self.activeBrowserJobs = MAX(0, self.activeBrowserJobs - 1);
        [self updateDockBadge];
        BOOL failed = [stage isEqualToString:@"error"];
        NSString *fallback = failed ? @"Failed" : @"Finished";
        NSString *message = [data[@"message"] isKindOfClass:NSString.class] ? data[@"message"] : fallback;
        [self.coreClient logAppLine:[NSString stringWithFormat:@"browser job %@: @%@ — %@",
                                     stage, username, message]];
        [self postNotificationWithTitle:[NSString stringWithFormat:@"@%@ %@", username,
                                         failed ? @"failed" : @"downloaded"]
                                   body:message];
        [NSApp requestUserAttention:NSInformationalRequest];
    }
}

- (void)applicationWillTerminate:(NSNotification *)notification {
    [self.coreClient stop];
}

// Menu-bar resident: closing the window keeps XDL (and its core) running so
// browser-extension jobs still work; reopen from the status item or Dock.
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
    return NO;
}

#pragma mark - Status item

- (void)setupStatusItem {
    self.statusItem = [NSStatusBar.systemStatusBar statusItemWithLength:NSSquareStatusItemLength];
    [self applyStatusItemIcon];
    NSMenu *menu = [[NSMenu alloc] init];
    menu.delegate = self;
    self.statusItem.menu = menu;
}

- (void)applyStatusItemIcon {
    BOOL busy = self.activeBrowserJobs > 0;
    NSImage *image = [NSImage imageWithSystemSymbolName:busy ? @"arrow.down.circle.fill" : @"arrow.down.circle"
                               accessibilityDescription:@"XDL"];
    NSImageSymbolConfiguration *config =
        [NSImageSymbolConfiguration configurationWithPointSize:15 weight:NSFontWeightMedium];
    image = [image imageWithSymbolConfiguration:config];
    image.template = YES; // let macOS tint for light/dark menu bars
    self.statusItem.button.image = image;
}

// Rebuild from scratch on every open — no incremental-update bugs.
- (void)menuNeedsUpdate:(NSMenu *)menu {
    [menu removeAllItems];

    NSString *coreState;
    switch (self.coreClient.state) {
        case XDLCoreStateReady: coreState = @"Core: running"; break;
        case XDLCoreStateStarting: coreState = @"Core: starting…"; break;
        default: coreState = @"Core: stopped"; break;
    }
    [menu addItemWithTitle:coreState action:nil keyEquivalent:@""];
    if (self.activeBrowserJobs > 0) {
        NSString *jobs = [NSString stringWithFormat:@"Downloading %ld browser job%@…",
                          (long)self.activeBrowserJobs, self.activeBrowserJobs == 1 ? @"" : @"s"];
        [menu addItemWithTitle:jobs action:nil keyEquivalent:@""];
    }
    [menu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *openItem = [menu addItemWithTitle:@"Open XDL" action:@selector(openMainWindow:) keyEquivalent:@""];
    openItem.target = self;
    NSMenuItem *folderItem = [menu addItemWithTitle:@"Open Download Folder"
                                             action:@selector(openDownloadFolderFromStatusItem:)
                                      keyEquivalent:@""];
    folderItem.target = self;
    [menu addItem:[NSMenuItem separatorItem]];
    [menu addItemWithTitle:@"Quit XDL" action:@selector(terminate:) keyEquivalent:@"q"];
}

- (void)openMainWindow:(id)sender {
    [self.mainWindowController showWindow:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (void)openDownloadFolderFromStatusItem:(id)sender {
    NSString *path = XDLSettings.shared.downloadPath;
    if (path.length == 0) {
        path = [NSHomeDirectory() stringByAppendingPathComponent:@"Pictures"];
    }
    [NSWorkspace.sharedWorkspace openURL:[NSURL fileURLWithPath:path]];
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
    if (!flag) {
        [self.mainWindowController showWindow:nil];
    }
    return YES;
}

// Without an Edit menu, ⌘C/V/X/A/Z never reach the first responder and text
// fields can't be pasted into.
- (void)setupMainMenu {
    NSMenu *mainMenu = [[NSMenu alloc] init];

    NSMenuItem *appItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:appItem];
    NSMenu *appMenu = [[NSMenu alloc] init];
    appItem.submenu = appMenu;
    [appMenu addItemWithTitle:@"About XDL"
                       action:@selector(orderFrontStandardAboutPanel:)
                keyEquivalent:@""];
    [appMenu addItem:[NSMenuItem separatorItem]];
    [appMenu addItemWithTitle:@"Hide XDL" action:@selector(hide:) keyEquivalent:@"h"];
    NSMenuItem *hideOthers = [appMenu addItemWithTitle:@"Hide Others"
                                                action:@selector(hideOtherApplications:)
                                         keyEquivalent:@"h"];
    hideOthers.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagOption;
    [appMenu addItemWithTitle:@"Show All" action:@selector(unhideAllApplications:) keyEquivalent:@""];
    [appMenu addItem:[NSMenuItem separatorItem]];
    [appMenu addItemWithTitle:@"Quit XDL" action:@selector(terminate:) keyEquivalent:@"q"];

    NSMenuItem *editItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:editItem];
    NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
    editItem.submenu = editMenu;
    // AppKit declares undo:/redo: nowhere; reference them by name.
    [editMenu addItemWithTitle:@"Undo" action:NSSelectorFromString(@"undo:") keyEquivalent:@"z"];
    NSMenuItem *redo = [editMenu addItemWithTitle:@"Redo"
                                           action:NSSelectorFromString(@"redo:")
                                    keyEquivalent:@"z"];
    redo.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
    [editMenu addItem:[NSMenuItem separatorItem]];
    [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
    [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
    [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
    [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];

    NSMenuItem *windowItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:windowItem];
    NSMenu *windowMenu = [[NSMenu alloc] initWithTitle:@"Window"];
    windowItem.submenu = windowMenu;
    [windowMenu addItemWithTitle:@"Minimize" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
    [windowMenu addItemWithTitle:@"Close" action:@selector(performClose:) keyEquivalent:@"w"];
    NSApp.windowsMenu = windowMenu;

    NSApp.mainMenu = mainMenu;
}

@end
