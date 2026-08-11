#import <Cocoa/Cocoa.h>
#import "XDLPageViewController.h"

NS_ASSUME_NONNULL_BEGIN

@class XDLCoreClient;

@interface XDLSettingsViewController : XDLPageViewController

- (instancetype)initWithClient:(XDLCoreClient *)client;

@end

NS_ASSUME_NONNULL_END
